// Package pgstore implements store.Store on PostgreSQL using pgx.
package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lakehouse-platform/lp/internal/store"
)

type Store struct {
	pool *pgxpool.Pool
}

var _ store.Store = (*Store)(nil)

// Open connects to databaseURL (e.g. the CloudNativePG app secret's "uri").
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Pool() *pgxpool.Pool            { return s.pool }
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
func (s *Store) Close()                         { s.pool.Close() }

// mapErr translates PostgreSQL errors into store sentinels.
func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505": // unique_violation
			return store.ErrAlreadyExists
		case "23503": // foreign_key_violation: parent row missing
			return store.ErrNotFound
		}
	}
	return err
}

// ---- tenants ----

const tenantCols = `id, name, isolation_tier, create_time`

func scanTenant(row pgx.Row) (*store.Tenant, error) {
	var t store.Tenant
	var tier string
	if err := row.Scan(&t.ID, &t.Name, &tier, &t.CreateTime); err != nil {
		return nil, mapErr(err)
	}
	t.IsolationTier = store.IsolationTier(tier)
	t.CreateTime = t.CreateTime.UTC()
	return &t, nil
}

func (s *Store) CreateTenant(ctx context.Context, t *store.Tenant) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO tenants (`+tenantCols+`) VALUES ($1, $2, $3, $4)`,
		t.ID, t.Name, string(t.IsolationTier), t.CreateTime)
	return mapErr(err)
}

func (s *Store) GetTenant(ctx context.Context, id string) (*store.Tenant, error) {
	return scanTenant(s.pool.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id = $1`, id))
}

func (s *Store) ListTenants(ctx context.Context) ([]*store.Tenant, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+tenantCols+` FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTenant)
}

// ---- workspaces ----

const workspaceCols = `id, tenant_id, name, namespace_prefix, bucket, storage_prefix, create_time`

func scanWorkspace(row pgx.Row) (*store.Workspace, error) {
	var w store.Workspace
	if err := row.Scan(&w.ID, &w.TenantID, &w.Name, &w.NamespacePrefix, &w.Bucket, &w.StoragePrefix, &w.CreateTime); err != nil {
		return nil, mapErr(err)
	}
	w.CreateTime = w.CreateTime.UTC()
	return &w, nil
}

func (s *Store) CreateWorkspace(ctx context.Context, w *store.Workspace) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workspaces (`+workspaceCols+`) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		w.ID, w.TenantID, w.Name, w.NamespacePrefix, w.Bucket, w.StoragePrefix, w.CreateTime)
	return mapErr(err)
}

func (s *Store) GetWorkspace(ctx context.Context, id string) (*store.Workspace, error) {
	return scanWorkspace(s.pool.QueryRow(ctx, `SELECT `+workspaceCols+` FROM workspaces WHERE id = $1`, id))
}

func (s *Store) ListWorkspaces(ctx context.Context, tenantID string) ([]*store.Workspace, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+workspaceCols+` FROM workspaces WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanWorkspace)
}

// ---- clusters ----

// clusterSpec is the jsonb "spec" column: sizing that is read and written as a unit.
type clusterSpec struct {
	Driver    store.NodeSpec  `json:"driver"`
	Worker    store.NodeSpec  `json:"worker"`
	AutoScale store.AutoScale `json:"autoscale"`
}

const clusterCols = `id, workspace_id, name, kind, runtime_version, spec, autoterminate_minutes,
	state, state_message, connect_url, creator, create_time, last_activity_time`

func scanCluster(row pgx.Row) (*store.Cluster, error) {
	var c store.Cluster
	var kind, state string
	var spec []byte
	if err := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &kind, &c.RuntimeVersion, &spec,
		&c.AutoterminateMinutes, &state, &c.StateMessage, &c.ConnectURL, &c.Creator,
		&c.CreateTime, &c.LastActivityTime); err != nil {
		return nil, mapErr(err)
	}
	var sp clusterSpec
	if err := json.Unmarshal(spec, &sp); err != nil {
		return nil, fmt.Errorf("pgstore: cluster %s: bad spec: %w", c.ID, err)
	}
	c.Kind, c.State = store.ClusterKind(kind), store.ClusterState(state)
	c.Driver, c.Worker, c.AutoScale = sp.Driver, sp.Worker, sp.AutoScale
	c.CreateTime, c.LastActivityTime = c.CreateTime.UTC(), c.LastActivityTime.UTC()
	return &c, nil
}

func specJSON(c *store.Cluster) ([]byte, error) {
	return json.Marshal(clusterSpec{Driver: c.Driver, Worker: c.Worker, AutoScale: c.AutoScale})
}

func (s *Store) CreateCluster(ctx context.Context, c *store.Cluster) error {
	spec, err := specJSON(c)
	if err != nil {
		return err
	}
	// tenant_id is denormalised from the workspace for tenant-scoped queries;
	// a missing workspace inserts zero rows.
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO clusters (`+clusterCols+`, tenant_id)
		SELECT $1, w.id, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, w.tenant_id
		FROM workspaces w WHERE w.id = $2`,
		c.ID, c.WorkspaceID, c.Name, string(c.Kind), c.RuntimeVersion, spec, c.AutoterminateMinutes,
		string(c.State), c.StateMessage, c.ConnectURL, c.Creator, c.CreateTime, c.LastActivityTime)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) GetCluster(ctx context.Context, workspaceID, id string) (*store.Cluster, error) {
	return scanCluster(s.pool.QueryRow(ctx,
		`SELECT `+clusterCols+` FROM clusters WHERE workspace_id = $1 AND id = $2`, workspaceID, id))
}

func (s *Store) ListClusters(ctx context.Context, workspaceID string) ([]*store.Cluster, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+clusterCols+` FROM clusters WHERE workspace_id = $1 ORDER BY create_time, id`, workspaceID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanCluster)
}

func (s *Store) ListActiveClusters(ctx context.Context) ([]*store.Cluster, error) {
	// Matches the partial index clusters_active_idx.
	rows, err := s.pool.Query(ctx, `SELECT `+clusterCols+` FROM clusters
		WHERE state IN ('CLUSTER_STATE_PENDING', 'CLUSTER_STATE_RUNNING', 'CLUSTER_STATE_TERMINATING')
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanCluster)
}

func (s *Store) UpdateCluster(ctx context.Context, c *store.Cluster) error {
	spec, err := specJSON(c)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE clusters SET name = $3, kind = $4, runtime_version = $5, spec = $6,
			autoterminate_minutes = $7, state = $8, state_message = $9, connect_url = $10,
			last_activity_time = $11
		WHERE workspace_id = $1 AND id = $2`,
		c.WorkspaceID, c.ID, c.Name, string(c.Kind), c.RuntimeVersion, spec,
		c.AutoterminateMinutes, string(c.State), c.StateMessage, c.ConnectURL, c.LastActivityTime)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func collect[T any](rows pgx.Rows, scan func(pgx.Row) (*T, error)) ([]*T, error) {
	defer rows.Close()
	out := []*T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
