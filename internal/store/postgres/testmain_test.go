package postgres_test

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

// testStore is shared across every test in this package: repo tests each
// create their own fresh users/groups/checklists (unique per test via
// uniqueName), so a single container/migration for the whole run is enough
// and much faster than spinning one up per test.
var testStore *postgres.Store

// testDSN lets tests open a raw *sql.DB connection when they need to inspect
// state the repo layer deliberately hides (e.g. soft-deleted rows).
var testDSN string

// testTenantID is the one tenant every test in this package creates its
// users/groups/templates/checklists under.
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
	testDSN = dsn

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

// waitForDB guards against a known race in the postgres testcontainers
// module: its readiness check can fire while Postgres is still mid-restart
// during initdb, so the first real connection attempt can see a reset.
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
