// Package config loads lp-server configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr string

	// DatabaseURL selects the PostgreSQL store; empty uses the in-memory store.
	DatabaseURL string
	// Migrate applies embedded schema migrations on startup.
	Migrate bool

	// AuthMode is "dev" (static bearer tokens) or "oidc" (JWTs from Keycloak).
	AuthMode     string
	DevTokens    map[string]string // token -> "subject:group1,group2"
	OIDCIssuer   string
	OIDCAudience string
	OIDCJWKSURL  string

	// ComputeBackend is "fake" (in-process simulation) or "kubernetes".
	ComputeBackend string
	ReconcileEvery time.Duration
	SparkImage     string
	RayImage       string
	IcebergCatalog string // Iceberg REST catalog URI
	// CatalogBackend is "none" (dev) or "lakekeeper".
	CatalogBackend  string
	LakekeeperURL   string
	WarehouseBucket string
	S3Endpoint      string
	S3Region        string
	S3AccessKey     string
	S3SecretKey     string
	DefaultIdleMins int
}

func Load() Config {
	return Config{
		HTTPAddr:        env("LP_HTTP_ADDR", ":8080"),
		DatabaseURL:     env("LP_DATABASE_URL", ""),
		Migrate:         env("LP_MIGRATE", "true") == "true",
		AuthMode:        env("LP_AUTH_MODE", "dev"),
		DevTokens:       parseDevTokens(env("LP_DEV_TOKENS", "dev-admin=admin:lp-admins")),
		OIDCIssuer:      env("LP_OIDC_ISSUER", ""),
		OIDCAudience:    env("LP_OIDC_AUDIENCE", "lp"),
		OIDCJWKSURL:     env("LP_OIDC_JWKS_URL", ""),
		ComputeBackend:  env("LP_COMPUTE_BACKEND", "fake"),
		ReconcileEvery:  envDuration("LP_RECONCILE_EVERY", 5*time.Second),
		SparkImage:      env("LP_SPARK_IMAGE", "lp/spark-runtime:3.5.3-lp1"),
		RayImage:        env("LP_RAY_IMAGE", "rayproject/ray:2.40.0-py311"),
		IcebergCatalog:  env("LP_ICEBERG_CATALOG_URI", "http://lp-lakekeeper.lp-system.svc:8181/catalog"),
		CatalogBackend:  env("LP_CATALOG_BACKEND", "none"),
		LakekeeperURL:   env("LP_LAKEKEEPER_URL", "http://lp-lakekeeper.lp-system.svc:8181"),
		WarehouseBucket: env("LP_WAREHOUSE_BUCKET", "lp-warehouse"),
		S3Endpoint:      env("LP_S3_ENDPOINT", "http://minio.lp-system.svc:9000"),
		S3Region:        env("LP_S3_REGION", "local"),
		S3AccessKey:     env("LP_S3_ACCESS_KEY", ""),
		S3SecretKey:     env("LP_S3_SECRET_KEY", ""),
		DefaultIdleMins: envInt("LP_DEFAULT_AUTOTERMINATE_MINUTES", 60),
	}
}

// parseDevTokens parses "tok1=alice:grpA,grpB;tok2=bob:grpC".
func parseDevTokens(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && k != "" {
			out[k] = v
		}
	}
	return out
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(k)); err == nil {
		return v
	}
	return def
}
