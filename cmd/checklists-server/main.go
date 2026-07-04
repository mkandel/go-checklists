// Command checklists-server runs the Checklists HTTP server.
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()

	migrateDB, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open db for migration: %v", err)
	}
	if err := postgres.Migrate(migrateDB); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	migrateDB.Close()

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	store := postgres.NewStore(pool)
	mux := api.NewMux(store.Users(), store.Sessions())

	log.Printf("checklists-server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
