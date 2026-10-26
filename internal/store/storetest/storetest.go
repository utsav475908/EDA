// Package storetest is a conformance suite every store.Store must pass.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lakehouse-platform/lp/internal/store"
)

// Run exercises s. It must start empty; IDs are random so suites can share
// a database as long as each run uses a fresh schema or truncated tables.
func Run(t *testing.T, s store.Store) {
	ctx := context.Background()
	// PostgreSQL stores microseconds; keep test timestamps representable.
	now := time.Now().UTC().Truncate(time.Microsecond)

	tA := &store.Tenant{ID: store.NewID("t-"), Name: "alpha-" + store.NewID(""), IsolationTier: store.IsolationSoft, CreateTime: now}
	tB := &store.Tenant{ID: store.NewID("t-"), Name: "beta-" + store.NewID(""), IsolationTier: store.IsolationHard, CreateTime: now}
	must(t, s.CreateTenant(ctx, tA))
	must(t, s.CreateTenant(ctx, tB))

	t.Run("tenants", func(t *testing.T) {
		dup := *tA
		dup.ID = store.NewID("t-")
		wantErr(t, s.CreateTenant(ctx, &dup), store.ErrAlreadyExists)

		got, err := s.GetTenant(ctx, tB.ID)
		must(t, err)
		if *got != *tB {
			t.Fatalf("GetTenant = %+v, want %+v", got, tB)
		}
		_, err = s.GetTenant(ctx, "t-missing")
		wantErr(t, err, store.ErrNotFound)

		list, err := s.ListTenants(ctx)
		must(t, err)
		if !containsID(list, func(x *store.Tenant) string { return x.ID }, tA.ID, tB.ID) {
			t.Fatalf("ListTenants missing created tenants: %+v", list)
		}
	})

	wsA := &store.Workspace{ID: store.NewID("w-"), TenantID: tA.ID, Name: "ml", Bucket: "lp-warehouse",
		CreateTime: now}
	wsA.NamespacePrefix, wsA.StoragePrefix = "ws-"+wsA.ID[2:], wsA.ID
	must(t, s.CreateWorkspace(ctx, wsA))

	t.Run("workspaces", func(t *testing.T) {
		dup := *wsA
		dup.ID, dup.NamespacePrefix, dup.StoragePrefix = store.NewID("w-"), "ws-dup"+store.NewID(""), store.NewID("p-")
		wantErr(t, s.CreateWorkspace(ctx, &dup), store.ErrAlreadyExists) // same tenant + name

		orphan := &store.Workspace{ID: store.NewID("w-"), TenantID: "t-missing", Name: "x",
			NamespacePrefix: "ws-orphan" + store.NewID(""), Bucket: "b", StoragePrefix: store.NewID("p-"), CreateTime: now}
		wantErr(t, s.CreateWorkspace(ctx, orphan), store.ErrNotFound)

		got, err := s.GetWorkspace(ctx, wsA.ID)
		must(t, err)
		if *got != *wsA {
			t.Fatalf("GetWorkspace = %+v, want %+v", got, wsA)
		}
		list, err := s.ListWorkspaces(ctx, tB.ID)
		must(t, err)
		if len(list) != 0 {
			t.Fatalf("tenant B should have no workspaces, got %+v", list)
		}
	})

	t.Run("clusters", func(t *testing.T) {
		c := &store.Cluster{
			ID: store.NewID("c-"), WorkspaceID: wsA.ID, Name: "etl", Kind: store.KindRay,
			RuntimeVersion:       "lp-1.0-ray2.40",
			Driver:               store.NodeSpec{CPU: "4", Memory: "16Gi"},
			Worker:               store.NodeSpec{CPU: "8", Memory: "64Gi", GPUs: 2},
			AutoScale:            store.AutoScale{MinWorkers: 1, MaxWorkers: 4},
			AutoterminateMinutes: 30, State: store.StatePending, StateMessage: "waiting",
			Creator: "alice", CreateTime: now, LastActivityTime: now,
		}
		must(t, s.CreateCluster(ctx, c))

		orphan := *c
		orphan.ID, orphan.WorkspaceID = store.NewID("c-"), "w-missing"
		wantErr(t, s.CreateCluster(ctx, &orphan), store.ErrNotFound)

		got, err := s.GetCluster(ctx, wsA.ID, c.ID)
		must(t, err)
		if *got != *c {
			t.Fatalf("GetCluster round-trip mismatch:\n got %+v\nwant %+v", got, c)
		}
		_, err = s.GetCluster(ctx, "w-other", c.ID)
		wantErr(t, err, store.ErrNotFound) // cluster IDs are scoped by workspace

		active, err := s.ListActiveClusters(ctx)
		must(t, err)
		if !containsID(active, func(x *store.Cluster) string { return x.ID }, c.ID) {
			t.Fatal("pending cluster missing from ListActiveClusters")
		}

		c.State, c.StateMessage, c.ConnectURL = store.StateTerminated, "", ""
		c.LastActivityTime = now.Add(time.Minute)
		must(t, s.UpdateCluster(ctx, c))
		got, err = s.GetCluster(ctx, wsA.ID, c.ID)
		must(t, err)
		if *got != *c {
			t.Fatalf("after update:\n got %+v\nwant %+v", got, c)
		}
		active, err = s.ListActiveClusters(ctx)
		must(t, err)
		if containsID(active, func(x *store.Cluster) string { return x.ID }, c.ID) {
			t.Fatal("terminated cluster still listed as active")
		}

		missing := *c
		missing.ID = "c-missing"
		wantErr(t, s.UpdateCluster(ctx, &missing), store.ErrNotFound)

		list, err := s.ListClusters(ctx, wsA.ID)
		must(t, err)
		if len(list) != 1 || list[0].ID != c.ID {
			t.Fatalf("ListClusters = %+v", list)
		}
	})
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantErr(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got error %v, want %v", err, want)
	}
}

func containsID[T any](xs []*T, id func(*T) string, want ...string) bool {
	seen := map[string]bool{}
	for _, x := range xs {
		seen[id(x)] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}
