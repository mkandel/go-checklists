// Command resetdb wipes every application table in the target database
// (schema/migrations untouched), for quickly getting back to a clean slate
// before re-seeding during local development. Never intended for production
// use.
package main

import (
	"context"
	"log"
	"os"

	"github.com/mkandel/go-checklists/internal/config"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cfg, err := config.Load(os.Args[1:], os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `TRUNCATE TABLE
		tenants, users, sessions, password_reset_tokens, groups, user_groups,
		templates, template_items, checklists, checklist_items, checklist_events,
		notifications
		RESTART IDENTITY CASCADE`)
	if err != nil {
		log.Fatalf("truncate: %v", err)
	}

	log.Print("database reset")
}
