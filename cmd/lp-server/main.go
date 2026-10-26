// lp-server is the Lakehouse Platform control plane. Phase 1 runs IAM and
// compute in one binary; services split out behind gRPC as they grow.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lakehouse-platform/lp/internal/api"
	"github.com/lakehouse-platform/lp/internal/auth"
	"github.com/lakehouse-platform/lp/internal/catalog"
	"github.com/lakehouse-platform/lp/internal/compute"
	"github.com/lakehouse-platform/lp/internal/config"
	"github.com/lakehouse-platform/lp/internal/iam"
	"github.com/lakehouse-platform/lp/internal/k8s"
	"github.com/lakehouse-platform/lp/internal/store"
	"github.com/lakehouse-platform/lp/internal/store/pgstore"
	"github.com/lakehouse-platform/lp/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backend, err := newBackend(cfg)
	if err != nil {
		return err
	}
	authn, err := newAuthenticator(cfg)
	if err != nil {
		return err
	}
	st, closeStore, err := newStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeStore()

	cat, err := newCatalog(cfg)
	if err != nil {
		return err
	}
	iamSvc := &iam.Service{Store: st, Backend: backend, Catalog: cat, WarehouseBucket: cfg.WarehouseBucket, Now: time.Now}
	computeSvc := &compute.Service{Store: st, Policy: compute.DefaultPolicy, Now: time.Now}
	rec := &compute.Reconciler{Store: st, Backend: backend, Now: time.Now, PendingTimeout: 15 * time.Minute}
	go rec.Run(ctx, cfg.ReconcileEvery)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           NewHandler(authn, iamSvc, computeSvc, readiness(st)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("lp-server listening", "addr", cfg.HTTPAddr, "auth", cfg.AuthMode, "backend", cfg.ComputeBackend)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// NewHandler wires the public REST API. Health endpoints are unauthenticated;
// ready reports whether dependencies (the database) are reachable.
func NewHandler(authn auth.Authenticator, iamSvc *iam.Service, computeSvc *compute.Service, ready func(context.Context) error) http.Handler {
	apiMux := http.NewServeMux()
	iamSvc.Register(apiMux)
	computeSvc.Register(apiMux)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			slog.WarnContext(ctx, "not ready", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	root.Handle("/api/", auth.Middleware(authn, apiMux))
	return api.Recover(api.Logging(root))
}

func newStore(ctx context.Context, cfg config.Config) (store.Store, func(), error) {
	if cfg.DatabaseURL == "" {
		slog.Warn("LP_DATABASE_URL not set: using in-memory store, data is lost on restart")
		return store.NewMemory(), func() {}, nil
	}
	pg, err := pgstore.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Migrate {
		if err := pgstore.Migrate(ctx, pg.Pool(), migrations.FS); err != nil {
			pg.Close()
			return nil, nil, err
		}
	}
	return pg, pg.Close, nil
}

// readiness pings stores that support it (PostgreSQL); others are always ready.
func readiness(st store.Store) func(context.Context) error {
	if p, ok := st.(interface{ Ping(context.Context) error }); ok {
		return p.Ping
	}
	return func(context.Context) error { return nil }
}

func newBackend(cfg config.Config) (k8s.Backend, error) {
	switch cfg.ComputeBackend {
	case "fake":
		return k8s.NewFakeBackend(), nil
	case "kubernetes":
		c, err := k8s.InCluster()
		if err != nil {
			return nil, err
		}
		return &k8s.KubeBackend{Client: c, Runtime: k8s.RuntimeConfig{
			SparkImage: cfg.SparkImage, RayImage: cfg.RayImage,
			IcebergCatalogURI: cfg.IcebergCatalog, S3Endpoint: cfg.S3Endpoint,
		}}, nil
	}
	return nil, errors.New("LP_COMPUTE_BACKEND must be fake or kubernetes")
}

func newCatalog(cfg config.Config) (catalog.Catalog, error) {
	switch cfg.CatalogBackend {
	case "none":
		return catalog.Noop{}, nil
	case "lakekeeper":
		return &catalog.Lakekeeper{BaseURL: cfg.LakekeeperURL, S3Endpoint: cfg.S3Endpoint, S3Region: cfg.S3Region,
			S3AccessKey: cfg.S3AccessKey, S3SecretKey: cfg.S3SecretKey}, nil
	}
	return nil, errors.New("LP_CATALOG_BACKEND must be none or lakekeeper")
}

func newAuthenticator(cfg config.Config) (auth.Authenticator, error) {
	switch cfg.AuthMode {
	case "dev":
		slog.Warn("dev auth enabled: static bearer tokens, do not use in production")
		return auth.NewDevAuthenticator(cfg.DevTokens), nil
	case "oidc":
		if cfg.OIDCIssuer == "" || cfg.OIDCJWKSURL == "" {
			return nil, errors.New("oidc auth requires LP_OIDC_ISSUER and LP_OIDC_JWKS_URL")
		}
		return &auth.OIDCAuthenticator{Issuer: cfg.OIDCIssuer, Audience: cfg.OIDCAudience, JWKSURL: cfg.OIDCJWKSURL}, nil
	}
	return nil, errors.New("LP_AUTH_MODE must be dev or oidc")
}
