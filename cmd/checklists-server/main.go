// Command checklists-server runs the Checklists HTTP server.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/mkandel/go-checklists/internal/web"
)

// sessionCleanupInterval is how often expired sessions are swept from the
// database; shutdownTimeout bounds how long graceful shutdown waits for
// in-flight requests to finish.
const (
	sessionCleanupInterval = time.Hour
	shutdownTimeout        = 10 * time.Second
)

// version is set at build time via:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/checklists-server
//
// and left as "dev" for plain `go build`/`go run`. internal/api and
// internal/web each get their own copy assigned in main below, rather than
// exporting a shared package, so this is the single place that needs to
// know about both.
var version = "dev"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("checklists-server %s starting", version)

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

	api.Version = version
	web.Version = version
	api.TrustProxy = cfg.TrustProxy

	apiMux := http.NewServeMux()
	api.RegisterRoutes(apiMux, store)
	apiHandler := api.WithAccessLog(api.WithSession(store, apiMux))

	webMux := http.NewServeMux()
	api.RegisterAuthRoutes(webMux, store)
	web.RegisterRoutes(webMux, store)
	webHandler := api.WithAccessLog(api.WithSession(store, webMux))

	var wg sync.WaitGroup
	wg.Add(1)
	go runSessionCleanup(ctx, &wg, store)
	wg.Add(1)
	go runPasswordResetTokenCleanup(ctx, &wg, store)
	wg.Add(1)
	go runEmailDelivery(ctx, &wg, store)

	apiSrv := &http.Server{Addr: cfg.APIAddr(), Handler: apiHandler}
	webSrv := &http.Server{Addr: cfg.WebAddr(), Handler: webHandler}

	serveErr := make(chan error, 2)
	go serve("api-server", apiSrv, serveErr)
	go serve("web-server", webSrv, serveErr)

	shutdownBoth := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := apiSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("api-server shutdown: %v", err)
		}
		if err := webSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("web-server shutdown: %v", err)
		}
	}

	select {
	case err := <-serveErr:
		if err != nil {
			shutdownBoth()
			log.Fatalf("serve: %v", err)
		}
	case <-ctx.Done():
		log.Print("shutting down...")
		shutdownBoth()
	}

	wg.Wait()
}

// serve runs srv until it's shut down or fails to start, reporting the
// outcome on serveErr (nil on a clean shutdown). name identifies the server
// in log output.
func serve(name string, srv *http.Server, serveErr chan<- error) {
	log.Printf("%s listening on %s", name, srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		serveErr <- fmt.Errorf("%s: %w", name, err)
		return
	}
	serveErr <- nil
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

// runPasswordResetTokenCleanup periodically deletes expired password-reset
// tokens until ctx is canceled, then signals wg it's done so main can exit
// cleanly. Shares sessionCleanupInterval with runSessionCleanup — there's no
// reason for these to sweep at different rates.
func runPasswordResetTokenCleanup(ctx context.Context, wg *sync.WaitGroup, store *postgres.Store) {
	defer wg.Done()

	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := store.PasswordResetTokens().DeleteExpired(ctx, time.Now())
			if err != nil {
				log.Printf("password reset token cleanup: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("password reset token cleanup: removed %d expired token(s)", n)
			}
		}
	}
}
