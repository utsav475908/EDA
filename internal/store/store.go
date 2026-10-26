package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

// Store is the platform metadata repository. The in-memory implementation
// backs dev mode and tests; production uses PostgreSQL (see migrations/).
type Store interface {
	CreateTenant(ctx context.Context, t *Tenant) error
	GetTenant(ctx context.Context, id string) (*Tenant, error)
	ListTenants(ctx context.Context) ([]*Tenant, error)

	CreateWorkspace(ctx context.Context, w *Workspace) error
	GetWorkspace(ctx context.Context, id string) (*Workspace, error)
	ListWorkspaces(ctx context.Context, tenantID string) ([]*Workspace, error)

	CreateCluster(ctx context.Context, c *Cluster) error
	GetCluster(ctx context.Context, workspaceID, id string) (*Cluster, error)
	ListClusters(ctx context.Context, workspaceID string) ([]*Cluster, error)
	// ListActiveClusters returns clusters across all workspaces that the
	// reconciler still needs to drive.
	ListActiveClusters(ctx context.Context) ([]*Cluster, error)
	UpdateCluster(ctx context.Context, c *Cluster) error
}

// NewID returns a short random identifier with the given prefix.
func NewID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
