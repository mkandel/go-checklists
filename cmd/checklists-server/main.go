// Command checklists-server runs the Checklists HTTP server.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/config"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// sessionCleanupInterval is how often expired sessions are swept from the
// database; shutdownTimeout bounds how long graceful shutdown waits for
// in-flight requests to finish.
const (
	sessionCleanupInterval = time.Hour
	shutdownTimeout        = 10 * time.Second
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	cfg, err := config.Load(os.Args[1:], os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	migrateDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db for migration: %v", err)
	}
	if err := postgres.Migrate(migrateDB); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	migrateDB.Close()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	store := postgres.NewStore(pool)
	if err := ensureDefaultTenant(ctx, store); err != nil {
		log.Fatalf("ensure default tenant: %v", err)
	}
	mux := api.NewMux(store)

	var wg sync.WaitGroup
	wg.Add(1)
	go runSessionCleanup(ctx, &wg, store)
	wg.Add(1)
	go runEmailDelivery(ctx, &wg, store)

	srv := &http.Server{Addr: cfg.Addr(), Handler: mux}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("checklists-server listening on %s", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatalf("serve: %v", err)
		}
	case <-ctx.Done():
		log.Print("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}

	wg.Wait()
}

// ensureDefaultTenant makes on-prem/standalone installs work with zero
// manual setup: if no tenant exists yet, it provisions the single default
// tenant that GetSoleTenant (and handleLogin) expect to find. A v2 SaaS
// deployment with self-service tenant signup would replace this.
func ensureDefaultTenant(ctx context.Context, store *postgres.Store) error {
	if _, err := store.Tenants().GetSoleTenant(ctx); err == nil {
		return nil
	}
	return store.Tenants().Create(ctx, &domain.Tenant{Name: "Default", Slug: "default"})
}

// runSessionCleanup periodically deletes expired sessions until ctx is
// canceled, then signals wg it's done so main can exit cleanly.
func runSessionCleanup(ctx context.Context, wg *sync.WaitGroup, store *postgres.Store) {
	defer wg.Done()

	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := store.Sessions().DeleteExpired(ctx, time.Now())
			if err != nil {
				log.Printf("session cleanup: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("session cleanup: removed %d expired session(s)", n)
			}
		}
	}
}
