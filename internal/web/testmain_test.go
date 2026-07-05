//go:build integration

package web_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// testStore and testTenantID mirror internal/api's identical TestMain
// pattern (one Postgres testcontainer + migration for the whole package
// run, one shared tenant every test creates its fixtures under).
var testStore *postgres.Store
var testTenantID int64

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("checklists_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
	)
	if err != nil {
		panic(err)
	}
	defer container.Terminate(ctx)

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	migrateDB, err := sql.Open("pgx", dsn)
	if err != nil {
		panic(err)
	}
	if err := waitForDB(migrateDB); err != nil {
		panic(err)
	}
	if err := postgres.Migrate(migrateDB); err != nil {
		panic(err)
	}
	migrateDB.Close()

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	testStore = postgres.NewStore(pool)

	tenant := &domain.Tenant{Name: "Test Tenant", Slug: "test-tenant"}
	if err := testStore.Tenants().Create(ctx, tenant); err != nil {
		panic(err)
	}
	testTenantID = tenant.ID

	return m.Run()
}

func waitForDB(db *sql.DB) error {
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if lastErr = db.Ping(); lastErr == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return lastErr
}
