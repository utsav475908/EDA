package store

import (
	"context"
	"sort"
	"sync"
)

type Memory struct {
	mu         sync.RWMutex
	tenants    map[string]*Tenant
	workspaces map[string]*Workspace
	clusters   map[string]*Cluster
}

func NewMemory() *Memory {
	return &Memory{
		tenants:    map[string]*Tenant{},
		workspaces: map[string]*Workspace{},
		clusters:   map[string]*Cluster{},
	}
}

func (m *Memory) CreateTenant(_ context.Context, t *Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.tenants {
		if e.Name == t.Name {
			return ErrAlreadyExists
		}
	}
	m.tenants[t.ID] = clone(t)
	return nil
}

func (m *Memory) GetTenant(_ context.Context, id string) (*Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(t), nil
}

func (m *Memory) ListTenants(_ context.Context) ([]*Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return sorted(m.tenants, func(*Tenant) bool { return true }, func(t *Tenant) string { return t.Name }), nil
}

func (m *Memory) CreateWorkspace(_ context.Context, w *Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tenants[w.TenantID]; !ok {
		return ErrNotFound
	}
	for _, e := range m.workspaces {
		if e.TenantID == w.TenantID && e.Name == w.Name {
			return ErrAlreadyExists
		}
	}
	m.workspaces[w.ID] = clone(w)
	return nil
}

func (m *Memory) GetWorkspace(_ context.Context, id string) (*Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.workspaces[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(w), nil
}

func (m *Memory) ListWorkspaces(_ context.Context, tenantID string) ([]*Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return sorted(m.workspaces, func(w *Workspace) bool { return w.TenantID == tenantID },
		func(w *Workspace) string { return w.Name }), nil
}

func (m *Memory) CreateCluster(_ context.Context, c *Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workspaces[c.WorkspaceID]; !ok {
		return ErrNotFound
	}
	m.clusters[c.ID] = clone(c)
	return nil
}

func (m *Memory) GetCluster(_ context.Context, workspaceID, id string) (*Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clusters[id]
	if !ok || c.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	return clone(c), nil
}

func (m *Memory) ListClusters(_ context.Context, workspaceID string) ([]*Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return sorted(m.clusters, func(c *Cluster) bool { return c.WorkspaceID == workspaceID },
		func(c *Cluster) string { return c.CreateTime.Format("20060102150405.000000000") + c.ID }), nil
}

func (m *Memory) ListActiveClusters(_ context.Context) ([]*Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return sorted(m.clusters, (*Cluster).Active, func(c *Cluster) string { return c.ID }), nil
}

func (m *Memory) UpdateCluster(_ context.Context, c *Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.clusters[c.ID]; !ok {
		return ErrNotFound
	}
	m.clusters[c.ID] = clone(c)
	return nil
}

func clone[T any](v *T) *T {
	c := *v
	return &c
}

func sorted[T any](m map[string]*T, keep func(*T) bool, key func(*T) string) []*T {
	out := make([]*T, 0, len(m))
	for _, v := range m {
		if keep(v) {
			out = append(out, clone(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return key(out[i]) < key(out[j]) })
	return out
}
