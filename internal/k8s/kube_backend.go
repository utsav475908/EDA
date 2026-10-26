package k8s

import (
	"context"
	"errors"
	"fmt"

	"github.com/lakehouse-platform/lp/internal/store"
)

// KubeBackend drives real clusters through the Kubernetes API.
type KubeBackend struct {
	Client  *Client
	Runtime RuntimeConfig
}

func (b *KubeBackend) EnsureWorkspace(ctx context.Context, ws *store.Workspace) error {
	for _, o := range WorkspaceManifests(ws) {
		if err := b.Client.Apply(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

func (b *KubeBackend) manifests(ws *store.Workspace, c *store.Cluster) []Obj {
	if c.Kind == store.KindRay {
		return []Obj{RayClusterManifest(ws, c, b.Runtime)}
	}
	return SparkConnectManifests(ws, c, b.Runtime)
}

func (b *KubeBackend) Apply(ctx context.Context, ws *store.Workspace, c *store.Cluster) error {
	for _, o := range b.manifests(ws, c) {
		if err := b.Client.Apply(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

func (b *KubeBackend) Observe(ctx context.Context, ws *store.Workspace, c *store.Cluster) (Observation, error) {
	ns, name := ComputeNamespace(ws), ResourceName(c)
	if c.Kind == store.KindRay {
		o, err := b.Client.Get(ctx, "ray.io/v1", "RayCluster", ns, name)
		if err != nil {
			return Observation{}, err
		}
		status, _ := o["status"].(Obj)
		state, _ := status["state"].(string)
		return Observation{
			Ready:      state == "ready",
			Failed:     state == "failed",
			Message:    fmt.Sprint(status["reason"]),
			ConnectURL: "ray://" + name + "-head-svc." + ns + ".svc.cluster.local:10001",
		}, nil
	}
	o, err := b.Client.Get(ctx, "apps/v1", "Deployment", ns, name+"-driver")
	if err != nil {
		return Observation{}, err
	}
	status, _ := o["status"].(Obj)
	ready, _ := status["readyReplicas"].(float64)
	obs := Observation{Ready: ready >= 1, ConnectURL: fmt.Sprintf("sc://%s:%d", connectHost(ws, c), sparkConnectPort)}
	conds, _ := status["conditions"].([]any)
	for _, raw := range conds {
		cond, _ := raw.(Obj)
		if cond["type"] == "Progressing" && cond["status"] == "False" {
			obs.Failed, obs.Message = true, fmt.Sprint(cond["message"])
		}
	}
	return obs, nil
}

func (b *KubeBackend) Delete(ctx context.Context, ws *store.Workspace, c *store.Cluster) (bool, error) {
	gone := true
	for _, o := range b.manifests(ws, c) {
		meta := o["metadata"].(Obj)
		av, kind := o["apiVersion"].(string), o["kind"].(string)
		ns, name := meta["namespace"].(string), meta["name"].(string)
		if err := b.Client.Delete(ctx, av, kind, ns, name); err != nil {
			return false, err
		}
		if _, err := b.Client.Get(ctx, av, kind, ns, name); !errors.Is(err, ErrNotFound) {
			gone = false
		}
	}
	return gone, nil
}
