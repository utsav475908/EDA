-- Platform metadata schema (database: lp_platform).
-- Iceberg, MLflow, JupyterHub and Ranger each get their own database.

CREATE TABLE tenants (
    id              text PRIMARY KEY,
    name            text NOT NULL UNIQUE,
    isolation_tier  text NOT NULL CHECK (isolation_tier IN ('ISOLATION_TIER_SOFT', 'ISOLATION_TIER_HARD')),
    create_time     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workspaces (
    id                text PRIMARY KEY,
    tenant_id         text NOT NULL REFERENCES tenants(id),
    name              text NOT NULL,
    namespace_prefix  text NOT NULL UNIQUE,
    bucket            text NOT NULL,
    storage_prefix    text NOT NULL,
    create_time       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name),
    UNIQUE (bucket, storage_prefix)
);

CREATE TABLE clusters (
    id                     text PRIMARY KEY,
    workspace_id           text NOT NULL REFERENCES workspaces(id),
    tenant_id              text NOT NULL REFERENCES tenants(id),
    name                   text NOT NULL,
    kind                   text NOT NULL,
    runtime_version        text NOT NULL,
    spec                   jsonb NOT NULL,          -- driver, worker, autoscale
    autoterminate_minutes  integer NOT NULL,
    state                  text NOT NULL,
    state_message          text NOT NULL DEFAULT '',
    connect_url            text NOT NULL DEFAULT '',
    creator                text NOT NULL,
    create_time            timestamptz NOT NULL DEFAULT now(),
    last_activity_time     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX clusters_workspace_idx ON clusters (workspace_id, create_time);
-- The reconciler scans only clusters that still hold resources.
CREATE INDEX clusters_active_idx ON clusters (state)
    WHERE state IN ('CLUSTER_STATE_PENDING', 'CLUSTER_STATE_RUNNING', 'CLUSTER_STATE_TERMINATING');

CREATE TABLE audit_events (
    id            bigserial PRIMARY KEY,
    ts            timestamptz NOT NULL DEFAULT now(),
    tenant_id     text,
    workspace_id  text,
    actor         text NOT NULL,
    action        text NOT NULL,
    resource      text NOT NULL,
    request       jsonb,
    result        text NOT NULL
);
CREATE INDEX audit_events_tenant_ts_idx ON audit_events (tenant_id, ts DESC);

-- Transactional outbox for async events (NATS publisher drains this).
CREATE TABLE outbox (
    id            bigserial PRIMARY KEY,
    topic         text NOT NULL,
    payload       jsonb NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    published_at  timestamptz
);

-- Defense in depth: tenant-scoped tables only expose rows for lp.tenant_id,
-- which lp-server sets per transaction (SET LOCAL lp.tenant_id = ...).
-- The reconciler connects as a role with BYPASSRLS.
ALTER TABLE workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE clusters ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workspaces
    USING (tenant_id = current_setting('lp.tenant_id', true));
CREATE POLICY tenant_isolation ON clusters
    USING (tenant_id = current_setting('lp.tenant_id', true));
