package k8s

import (
	"context"
	"fmt"
	"sync"

	"github.com/lakehouse-platform/lp/internal/store"
)

// FakeBackend simulates cluster lifecycles in-process for local dev and tests.
// A cluster becomes ready after StartupObservations calls to Observe.
type FakeBackend struct {
	StartupObservations int

	mu         sync.Mutex
	workspaces map[string]bool
	clusters   map[string]int // id -> observe count
}

func NewFakeBackend() *FakeBackend {
	return &FakeBackend{StartupObservations: 2, workspaces: map[string]bool{}, clusters: map[string]int{}}
}

func (f *FakeBackend) EnsureWorkspace(_ context.Context, ws *store.Workspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workspaces[ws.ID] = true
	return nil
}

func (f *FakeBackend) Apply(_ context.Context, ws *store.Workspace, c *store.Cluster) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.workspaces[ws.ID] {
		return fmt.Errorf("workspace %s not provisioned", ws.ID)
	}
	if _, ok := f.clusters[c.ID]; !ok {
		f.clusters[c.ID] = 0
	}
	return nil
}

func (f *FakeBackend) Observe(_ context.Context, ws *store.Workspace, c *store.Cluster) (Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.clusters[c.ID]
	if !ok {
		return Observation{}, ErrNotFound
	}
	f.clusters[c.ID] = n + 1
	url := fmt.Sprintf("sc://%s:%d", connectHost(ws, c), sparkConnectPort)
	if c.Kind == store.KindRay {
		url = "ray://" + ResourceName(c) + "-head-svc:10001"
	}
	return Observation{Ready: n+1 >= f.StartupObservations, ConnectURL: url}, nil
}

func (f *FakeBackend) Delete(_ context.Context, _ *store.Workspace, c *store.Cluster) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.clusters, c.ID)
	return true, nil
}

// Running reports how many clusters currently hold simulated resources.
func (f *FakeBackend) Running() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.clusters)
}
