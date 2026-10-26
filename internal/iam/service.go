// Package iam manages the tenant -> workspace hierarchy.
package iam

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/lakehouse-platform/lp/internal/api"
	"github.com/lakehouse-platform/lp/internal/auth"
	"github.com/lakehouse-platform/lp/internal/catalog"
	"github.com/lakehouse-platform/lp/internal/k8s"
	"github.com/lakehouse-platform/lp/internal/store"
)

// DNS-1123 label, short enough to fit "<prefix>-notebooks" style names.
var nameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

type Service struct {
	Store   store.Store
	Backend k8s.Backend
	Catalog catalog.Catalog
	// WarehouseBucket holds every workspace's Iceberg data under its own prefix.
	WarehouseBucket string
	Now             func() time.Time
}

func (s *Service) CreateTenant(ctx context.Context, p *auth.Principal, name string, tier store.IsolationTier) (*store.Tenant, error) {
	if !p.IsAdmin() {
		return nil, api.Errorf(api.CodePermissionDenied, "only platform admins can create tenants")
	}
	if !nameRE.MatchString(name) {
		return nil, api.Errorf(api.CodeInvalidArgument, "name must be a lowercase DNS label (max 32 chars)")
	}
	if tier == "" {
		tier = store.IsolationSoft
	}
	if tier != store.IsolationSoft && tier != store.IsolationHard {
		return nil, api.Errorf(api.CodeInvalidArgument, "unknown isolation tier")
	}
	t := &store.Tenant{ID: store.NewID("t-"), Name: name, IsolationTier: tier, CreateTime: s.Now().UTC()}
	if err := s.Store.CreateTenant(ctx, t); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "audit", "action", "tenant.create", "actor", p.Subject, "tenant", t.ID)
	return t, nil
}

func (s *Service) ListTenants(ctx context.Context, p *auth.Principal) ([]*store.Tenant, error) {
	all, err := s.Store.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, t := range all {
		if p.CanAccessTenant(t.ID) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *Service) CreateWorkspace(ctx context.Context, p *auth.Principal, tenantID, name string) (*store.Workspace, error) {
	if !p.CanAccessTenant(tenantID) {
		return nil, api.Errorf(api.CodeNotFound, "tenant not found")
	}
	if !nameRE.MatchString(name) {
		return nil, api.Errorf(api.CodeInvalidArgument, "name must be a lowercase DNS label (max 32 chars)")
	}
	if _, err := s.Store.GetTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	id := store.NewID("w-")
	short := strings.TrimPrefix(id, "w-")
	ws := &store.Workspace{
		ID: id, TenantID: tenantID, Name: name,
		NamespacePrefix: "ws-" + short,
		Bucket:          s.WarehouseBucket,
		StoragePrefix:   id,
		CreateTime:      s.Now().UTC(),
	}
	// Provision infrastructure first so a stored workspace is always usable.
	// Both steps are idempotent, so a failed create can simply be retried.
	if err := s.Backend.EnsureWorkspace(ctx, ws); err != nil {
		return nil, err
	}
	if err := s.Catalog.CreateWarehouse(ctx, ws); err != nil {
		return nil, err
	}
	if err := s.Store.CreateWorkspace(ctx, ws); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "audit", "action", "workspace.create", "actor", p.Subject, "workspace", ws.ID)
	return ws, nil
}

func (s *Service) ListWorkspaces(ctx context.Context, p *auth.Principal, tenantID string) ([]*store.Workspace, error) {
	if !p.CanAccessTenant(tenantID) {
		return nil, api.Errorf(api.CodeNotFound, "tenant not found")
	}
	return s.Store.ListWorkspaces(ctx, tenantID)
}
