// Package k8s turns platform objects into Kubernetes resources.
//
// Interactive Spark clusters are a Spark Connect server Deployment whose
// executors are launched dynamically as pods in the same namespace. Ray
// clusters are KubeRay RayCluster CRs.
package k8s

import (
	"context"

	"github.com/lakehouse-platform/lp/internal/store"
)

// Observation is what the reconciler learns about a cluster's live state.
type Observation struct {
	Ready      bool
	Failed     bool
	Message    string
	ConnectURL string
}

type Backend interface {
	// EnsureWorkspace creates the namespaces, service accounts, RBAC and
	// quotas a workspace needs. Must be idempotent.
	EnsureWorkspace(ctx context.Context, ws *store.Workspace) error
	Apply(ctx context.Context, ws *store.Workspace, c *store.Cluster) error
	Observe(ctx context.Context, ws *store.Workspace, c *store.Cluster) (Observation, error)
	// Delete removes the cluster's resources; it returns true once they are gone.
	Delete(ctx context.Context, ws *store.Workspace, c *store.Cluster) (bool, error)
}

func ComputeNamespace(ws *store.Workspace) string  { return ws.NamespacePrefix + "-compute" }
func NotebookNamespace(ws *store.Workspace) string { return ws.NamespacePrefix + "-notebooks" }
func ResourceName(c *store.Cluster) string         { return "lp-" + c.ID }
func connectHost(ws *store.Workspace, c *store.Cluster) string {
	return ResourceName(c) + "-driver." + ComputeNamespace(ws) + ".svc.cluster.local"
}
