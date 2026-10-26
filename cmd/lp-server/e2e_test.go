package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/lakehouse-platform/lp/internal/auth"
	"github.com/lakehouse-platform/lp/internal/catalog"
	"github.com/lakehouse-platform/lp/internal/compute"
	"github.com/lakehouse-platform/lp/internal/iam"
	"github.com/lakehouse-platform/lp/internal/k8s"
	"github.com/lakehouse-platform/lp/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type harness struct {
	t       *testing.T
	srv     *httptest.Server
	rec     *compute.Reconciler
	backend *k8s.FakeBackend
	clock   *clock
}

func newHarness(t *testing.T, tokens map[string]string) *harness {
	clk := &clock{t: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)}
	st := store.NewMemory()
	be := k8s.NewFakeBackend()
	h := &harness{
		t: t, backend: be, clock: clk,
		rec: &compute.Reconciler{Store: st, Backend: be, Now: clk.Now, PendingTimeout: 10 * time.Minute},
	}
	h.srv = httptest.NewServer(NewHandler(
		auth.NewDevAuthenticator(tokens),
		&iam.Service{Store: st, Backend: be, Catalog: catalog.Noop{}, WarehouseBucket: "lp-warehouse", Now: clk.Now},
		&compute.Service{Store: st, Policy: compute.DefaultPolicy, Now: clk.Now},
		readiness(st),
	))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) do(token, method, path string, body any, out any) int {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			h.t.Fatal(err)
		}
	}
	return resp.StatusCode
}

func TestTenantIsolationAndClusterLifecycle(t *testing.T) {
	// The admin creates tenants first; tenant IDs are random, so per-tenant
	// user tokens are wired up afterwards on a second server sharing the store.
	tokens := map[string]string{"admin": "root:lp-admins"}
	h := newHarness(t, tokens)

	if code := h.do("", "GET", "/api/v1/tenants", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous request: got %d, want 401", code)
	}

	var ta, tb store.Tenant
	if code := h.do("admin", "POST", "/api/v1/tenants", map[string]any{"name": "analytics"}, &ta); code != 200 {
		t.Fatalf("create tenant: %d", code)
	}
	h.do("admin", "POST", "/api/v1/tenants", map[string]any{"name": "finance"}, &tb)
	if code := h.do("admin", "POST", "/api/v1/tenants", map[string]any{"name": "finance"}, nil); code != http.StatusConflict {
		t.Fatalf("duplicate tenant: got %d, want 409", code)
	}
	if code := h.do("admin", "POST", "/api/v1/tenants", map[string]any{"name": "Bad_Name"}, nil); code != http.StatusBadRequest {
		t.Fatalf("invalid name: got %d, want 400", code)
	}

	// Re-wire auth now that tenant IDs are known.
	h2 := newHarnessShared(t, h, map[string]string{
		"admin": "root:lp-admins",
		"alice": "alice:" + auth.TenantGroupPrefix + ta.ID,
		"bob":   "bob:" + auth.TenantGroupPrefix + tb.ID,
	})

	if code := h2.do("alice", "POST", "/api/v1/tenants", map[string]any{"name": "rogue"}, nil); code != http.StatusForbidden {
		t.Fatalf("non-admin tenant create: got %d, want 403", code)
	}
	var list struct{ Tenants []store.Tenant }
	h2.do("alice", "GET", "/api/v1/tenants", nil, &list)
	if len(list.Tenants) != 1 || list.Tenants[0].ID != ta.ID {
		t.Fatalf("alice should see only her tenant, got %+v", list.Tenants)
	}

	var ws store.Workspace
	if code := h2.do("alice", "POST", "/api/v1/tenants/"+ta.ID+"/workspaces", map[string]any{"name": "ml-research"}, &ws); code != 200 {
		t.Fatalf("create workspace: %d", code)
	}
	if code := h2.do("bob", "POST", "/api/v1/tenants/"+ta.ID+"/workspaces", map[string]any{"name": "sneaky"}, nil); code != http.StatusNotFound {
		t.Fatalf("cross-tenant workspace create: got %d, want 404", code)
	}

	clusters := "/api/v1/workspaces/" + ws.ID + "/clusters"
	var c store.Cluster
	code := h2.do("alice", "POST", clusters, map[string]any{"cluster": map[string]any{
		"name": "etl", "autoscale": map[string]any{"minWorkers": 1, "maxWorkers": 4}, "autoterminateMinutes": 30,
	}}, &c)
	if code != 200 || c.State != store.StatePending || c.Kind != store.KindSpark || c.Creator != "alice" {
		t.Fatalf("create cluster: code=%d cluster=%+v", code, c)
	}
	if code := h2.do("bob", "GET", clusters+"/"+c.ID, nil, nil); code != http.StatusNotFound {
		t.Fatalf("cross-tenant cluster read: got %d, want 404", code)
	}
	if code := h2.do("alice", "POST", clusters, map[string]any{"cluster": map[string]any{
		"name": "huge", "autoscale": map[string]any{"maxWorkers": 500},
	}}, nil); code != http.StatusBadRequest {
		t.Fatalf("policy violation: got %d, want 400", code)
	}

	ctx := context.Background()
	get := func() store.Cluster {
		var out store.Cluster
		h2.do("alice", "GET", clusters+"/"+c.ID, nil, &out)
		return out
	}

	h.rec.ReconcileAll(ctx)
	if s := get().State; s != store.StatePending {
		t.Fatalf("after 1 reconcile: %s", s)
	}
	h.rec.ReconcileAll(ctx)
	running := get()
	if running.State != store.StateRunning || running.ConnectURL == "" {
		t.Fatalf("expected RUNNING with connect URL, got %+v", running)
	}

	// Heartbeats keep it alive past the idle window.
	h.clock.Advance(20 * time.Minute)
	if code := h2.do("alice", "POST", clusters+"/"+c.ID+"/heartbeat", nil, nil); code != 200 {
		t.Fatalf("heartbeat: %d", code)
	}
	h.clock.Advance(20 * time.Minute)
	h.rec.ReconcileAll(ctx)
	if s := get().State; s != store.StateRunning {
		t.Fatalf("cluster with recent heartbeat should stay running, got %s", s)
	}

	// Idle past autoterminateMinutes -> TERMINATING -> TERMINATED.
	h.clock.Advance(31 * time.Minute)
	h.rec.ReconcileAll(ctx)
	if s := get().State; s != store.StateTerminating {
		t.Fatalf("expected auto-termination, got %s", s)
	}
	h.rec.ReconcileAll(ctx)
	if got := get(); got.State != store.StateTerminated || got.ConnectURL != "" {
		t.Fatalf("expected TERMINATED, got %+v", got)
	}
	if n := h.backend.Running(); n != 0 {
		t.Fatalf("backend still holds %d clusters", n)
	}
}

// newHarnessShared serves the same store/backend/clock as base with new tokens.
func newHarnessShared(t *testing.T, base *harness, tokens map[string]string) *harness {
	st := base.rec.Store
	h := &harness{t: t, rec: base.rec, backend: base.backend, clock: base.clock}
	h.srv = httptest.NewServer(NewHandler(
		auth.NewDevAuthenticator(tokens),
		&iam.Service{Store: st, Backend: base.backend, Catalog: catalog.Noop{}, WarehouseBucket: "lp-warehouse", Now: base.clock.Now},
		&compute.Service{Store: st, Policy: compute.DefaultPolicy, Now: base.clock.Now},
		readiness(st),
	))
	t.Cleanup(h.srv.Close)
	return h
}
