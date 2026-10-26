package pgstore

import (
	"context"
	"os"
	"testing"

	"github.com/lakehouse-platform/lp/internal/store/storetest"
	"github.com/lakehouse-platform/lp/migrations"
)

// Requires a disposable database, e.g.:
//
//	docker run -d --name lp-pg -e POSTGRES_PASSWORD=lp -p 5432:5432 postgres:16
//	LP_TEST_DATABASE_URL=postgres://postgres:lp@localhost:5432/postgres?sslmode=disable go test ./internal/store/pgstore
//
// The test drops and recreates the "public" schema.
func TestPostgresConformance(t *testing.T) {
	url := os.Getenv("LP_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LP_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.Pool().Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, s.Pool(), migrations.FS); err != nil {
		t.Fatal(err)
	}
	// Re-running must be a no-op.
	if err := Migrate(ctx, s.Pool(), migrations.FS); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	storetest.Run(t, s)
}
