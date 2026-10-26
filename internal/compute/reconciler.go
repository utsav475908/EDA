package compute

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/lakehouse-platform/lp/internal/k8s"
	"github.com/lakehouse-platform/lp/internal/store"
)

// Reconciler drives every active cluster toward its desired state:
//
//	PENDING     -> apply manifests, wait for readiness -> RUNNING | ERROR
//	RUNNING     -> idle past autoterminate             -> TERMINATING
//	TERMINATING -> delete resources                    -> TERMINATED
type Reconciler struct {
	Store   store.Store
	Backend k8s.Backend
	Now     func() time.Time
	// PendingTimeout bounds how long a cluster may take to become ready.
	PendingTimeout time.Duration
}

func (r *Reconciler) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		r.ReconcileAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r *Reconciler) ReconcileAll(ctx context.Context) {
	clusters, err := r.Store.ListActiveClusters(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "reconcile: list clusters", "err", err)
		return
	}
	for _, c := range clusters {
		if err := r.reconcile(ctx, c); err != nil {
			slog.WarnContext(ctx, "reconcile cluster", "cluster", c.ID, "state", c.State, "err", err)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context, c *store.Cluster) error {
	ws, err := r.Store.GetWorkspace(ctx, c.WorkspaceID)
	if err != nil {
		return err
	}
	before := *c
	now := r.Now().UTC()

	switch c.State {
	case store.StatePending:
		if err := r.Backend.Apply(ctx, ws, c); err != nil {
			c.StateMessage = "apply failed: " + err.Error()
			break
		}
		obs, err := r.Backend.Observe(ctx, ws, c)
		switch {
		case err != nil && !errors.Is(err, k8s.ErrNotFound):
			c.StateMessage = "observe failed: " + err.Error()
		case obs.Failed:
			c.State, c.StateMessage = store.StateError, obs.Message
		case obs.Ready:
			c.State, c.StateMessage, c.ConnectURL = store.StateRunning, "", obs.ConnectURL
			c.LastActivityTime = now
		case r.PendingTimeout > 0 && now.Sub(c.CreateTime) > r.PendingTimeout:
			c.State, c.StateMessage = store.StateError, "timed out waiting for cluster to become ready"
		}
		// An errored cluster must not keep holding resources.
		if c.State == store.StateError {
			if _, err := r.Backend.Delete(ctx, ws, c); err != nil {
				slog.WarnContext(ctx, "cleanup errored cluster", "cluster", c.ID, "err", err)
			}
		}

	case store.StateRunning:
		idle := time.Duration(c.AutoterminateMinutes) * time.Minute
		if c.AutoterminateMinutes > 0 && now.Sub(c.LastActivityTime) > idle {
			c.State, c.StateMessage = store.StateTerminating, "auto-terminated after inactivity"
			slog.InfoContext(ctx, "audit", "action", "cluster.autoterminate", "actor", "system", "cluster", c.ID)
		}

	case store.StateTerminating:
		gone, err := r.Backend.Delete(ctx, ws, c)
		if err != nil {
			return err
		}
		if gone {
			c.State, c.ConnectURL = store.StateTerminated, ""
		}
	}

	if *c != before {
		return r.Store.UpdateCluster(ctx, c)
	}
	return nil
}
