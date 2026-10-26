// Package compute manages interactive Spark Connect and Ray clusters.
// API calls only record desired state; the Reconciler converges Kubernetes.
package compute

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/lakehouse-platform/lp/internal/api"
	"github.com/lakehouse-platform/lp/internal/auth"
	"github.com/lakehouse-platform/lp/internal/store"
)

// Policy bounds what users may request. Phase 2 makes these per-workspace
// ClusterPolicy objects managed by admins.
type Policy struct {
	MaxWorkers           int32
	MaxCPUPerNode        int
	MaxGPUsPerNode       int32
	DefaultAutoterminate int32
	RuntimeVersions      []string
}

var DefaultPolicy = Policy{
	MaxWorkers:           20,
	MaxCPUPerNode:        16,
	MaxGPUsPerNode:       8,
	DefaultAutoterminate: 60,
	RuntimeVersions:      []string{"lp-1.0-spark3.5", "lp-1.0-ray2.40"},
}

type Service struct {
	Store  store.Store
	Policy Policy
	Now    func() time.Time
}

// workspace resolves a workspace the caller may access. Inaccessible
// workspaces are reported as not found to avoid leaking their existence.
func (s *Service) workspace(ctx context.Context, p *auth.Principal, id string) (*store.Workspace, error) {
	ws, err := s.Store.GetWorkspace(ctx, id)
	if err != nil || !p.CanAccessTenant(ws.TenantID) {
		return nil, api.Errorf(api.CodeNotFound, "workspace not found")
	}
	return ws, nil
}

func (s *Service) CreateCluster(ctx context.Context, p *auth.Principal, workspaceID string, in store.Cluster) (*store.Cluster, error) {
	if _, err := s.workspace(ctx, p, workspaceID); err != nil {
		return nil, err
	}
	c := in
	if err := s.applyDefaultsAndValidate(&c); err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	c.ID = store.NewID("c-")
	c.WorkspaceID = workspaceID
	c.State = store.StatePending
	c.StateMessage = "waiting for resources"
	c.ConnectURL = ""
	c.Creator = p.Subject
	c.CreateTime, c.LastActivityTime = now, now
	if err := s.Store.CreateCluster(ctx, &c); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "audit", "action", "cluster.create", "actor", p.Subject, "workspace", workspaceID, "cluster", c.ID)
	return &c, nil
}

func (s *Service) applyDefaultsAndValidate(c *store.Cluster) error {
	pol := s.Policy
	if c.Name == "" {
		return api.Errorf(api.CodeInvalidArgument, "cluster.name is required")
	}
	if c.Kind == "" {
		c.Kind = store.KindSpark
	}
	if c.Kind != store.KindSpark && c.Kind != store.KindRay {
		return api.Errorf(api.CodeInvalidArgument, "unknown cluster kind")
	}
	if c.RuntimeVersion == "" {
		c.RuntimeVersion = pol.RuntimeVersions[0]
		if c.Kind == store.KindRay && len(pol.RuntimeVersions) > 1 {
			c.RuntimeVersion = pol.RuntimeVersions[1]
		}
	}
	if !contains(pol.RuntimeVersions, c.RuntimeVersion) {
		return api.Errorf(api.CodeInvalidArgument, "runtime version not allowed by policy")
	}
	for _, n := range []*store.NodeSpec{&c.Driver, &c.Worker} {
		if n.CPU == "" {
			n.CPU = "2"
		}
		if n.Memory == "" {
			n.Memory = "8Gi"
		}
		cpu, err := strconv.Atoi(n.CPU)
		if err != nil || cpu < 1 || cpu > pol.MaxCPUPerNode {
			return api.Errorf(api.CodeInvalidArgument, "cpu must be a whole number between 1 and "+strconv.Itoa(pol.MaxCPUPerNode))
		}
		if !strings.HasSuffix(n.Memory, "Gi") && !strings.HasSuffix(n.Memory, "Mi") {
			return api.Errorf(api.CodeInvalidArgument, "memory must be expressed in Gi or Mi")
		}
		if n.GPUs < 0 || n.GPUs > pol.MaxGPUsPerNode {
			return api.Errorf(api.CodeInvalidArgument, "gpus out of policy range")
		}
		if n.GPUs > 0 && c.Kind != store.KindRay {
			return api.Errorf(api.CodeInvalidArgument, "GPUs are only supported on Ray clusters in this release")
		}
	}
	if c.AutoScale.MaxWorkers == 0 {
		c.AutoScale.MaxWorkers = max(c.AutoScale.MinWorkers, 2)
	}
	a := c.AutoScale
	if a.MinWorkers < 0 || a.MaxWorkers < a.MinWorkers || a.MaxWorkers > pol.MaxWorkers {
		return api.Errorf(api.CodeInvalidArgument, "autoscale must satisfy 0 <= min <= max <= "+strconv.Itoa(int(pol.MaxWorkers)))
	}
	if c.AutoterminateMinutes == 0 {
		c.AutoterminateMinutes = pol.DefaultAutoterminate
	}
	if c.AutoterminateMinutes < 0 {
		return api.Errorf(api.CodeInvalidArgument, "autoterminateMinutes must be positive")
	}
	return nil
}

func (s *Service) GetCluster(ctx context.Context, p *auth.Principal, workspaceID, id string) (*store.Cluster, error) {
	if _, err := s.workspace(ctx, p, workspaceID); err != nil {
		return nil, err
	}
	return s.Store.GetCluster(ctx, workspaceID, id)
}

func (s *Service) ListClusters(ctx context.Context, p *auth.Principal, workspaceID string) ([]*store.Cluster, error) {
	if _, err := s.workspace(ctx, p, workspaceID); err != nil {
		return nil, err
	}
	return s.Store.ListClusters(ctx, workspaceID)
}

func (s *Service) TerminateCluster(ctx context.Context, p *auth.Principal, workspaceID, id string) error {
	c, err := s.GetCluster(ctx, p, workspaceID, id)
	if err != nil {
		return err
	}
	if c.State == store.StateTerminated || c.State == store.StateTerminating {
		return nil
	}
	c.State, c.StateMessage = store.StateTerminating, "terminated by "+p.Subject
	slog.InfoContext(ctx, "audit", "action", "cluster.terminate", "actor", p.Subject, "cluster", c.ID)
	return s.Store.UpdateCluster(ctx, c)
}

// Heartbeat records activity (called by the notebook kernel extension and the
// jobs service) so the idle reaper does not terminate a cluster in use.
func (s *Service) Heartbeat(ctx context.Context, p *auth.Principal, workspaceID, id string) error {
	c, err := s.GetCluster(ctx, p, workspaceID, id)
	if err != nil {
		return err
	}
	if c.State != store.StateRunning && c.State != store.StatePending {
		return api.Errorf(api.CodeFailedPrecondition, "cluster is not running")
	}
	c.LastActivityTime = s.Now().UTC()
	return s.Store.UpdateCluster(ctx, c)
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
