//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// waitForConnection guards against a known race in the postgres
// testcontainers module: its readiness check can fire while Postgres is
// still mid-restart during initdb, so the first real connection attempt can
// see a reset. Retry briefly rather than failing on that one attempt.
func waitForConnection(t *testing.T, db *sql.DB) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if lastErr = db.Ping(); lastErr == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("db did not become reachable: %v", lastErr)
}

// TestMigrate proves the whole local integration-test pipeline: spin up a
// real Postgres in a throwaway container, run the embedded migrations
// against it, and confirm the schema is queryable.
func TestMigrate(t *testing.T) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("checklists_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	waitForConnection(t, db)

	if err := postgres.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT count(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 users in fresh schema, got %d", count)
	}
}
