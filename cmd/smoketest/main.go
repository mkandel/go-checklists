// Command smoketest exercises the full HTTP surface end-to-end against a
// real Postgres database: login, create an ad-hoc checklist, check its
// items, and confirm the status transitions to complete. It runs the app's
// mux in-process (httptest.NewServer) against whatever DATABASE_URL points
// at, so it needs no separately-running server — just a reachable, migrated
// Postgres (see the "Smoke Test" GoLand run config). Reports PASS/FAIL via
// log output and exit code.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/config"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

const smokePassword = "smoketest-password"

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	if err := run(); err != nil {
		log.Fatalf("SMOKE TEST FAILED: %v", err)
	}
	log.Print("SMOKE TEST PASSED")
}

func run() error {
	cfg, err := config.Load(os.Args[1:], os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()

	migrateDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	if err := postgres.Migrate(migrateDB); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	migrateDB.Close()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	store := postgres.NewStore(pool)

	tenant, err := store.Tenants().GetSoleTenant(ctx)
	if err != nil {
		tenant = &domain.Tenant{Name: "Default", Slug: "default"}
		if err := store.Tenants().Create(ctx, tenant); err != nil {
			return fmt.Errorf("create default tenant: %w", err)
		}
	}

	username := fmt.Sprintf("smoketest-%d", time.Now().UnixNano())
	hash, err := auth.HashPassword(smokePassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user := &domain.User{TenantID: tenant.ID, Name: "Smoke Test", Username: username, PasswordHash: hash, IsActive: true}
	if err := store.Users().Create(ctx, user); err != nil {
		return fmt.Errorf("create smoke test user: %w", err)
	}

	srv := httptest.NewServer(api.NewMux(store))
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("cookie jar: %w", err)
	}
	client := &http.Client{Jar: jar}

	log.Printf("logging in as %s...", username)
	if err := login(client, srv.URL, username, smokePassword); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	log.Print("creating ad-hoc checklist...")
	checklist, err := createChecklist(client, srv.URL, user.ID)
	if err != nil {
		return fmt.Errorf("create checklist: %w", err)
	}
	if checklist.Status != domain.StatusOpen {
		return fmt.Errorf("expected new checklist status %q, got %q", domain.StatusOpen, checklist.Status)
	}
	if len(checklist.Items) != 2 {
		return fmt.Errorf("expected 2 items, got %d", len(checklist.Items))
	}

	log.Print("checking items...")
	for _, item := range checklist.Items {
		if err := checkItem(client, srv.URL, checklist.ID, item.ID); err != nil {
			return fmt.Errorf("check item %d: %w", item.ID, err)
		}
	}

	log.Print("verifying completion...")
	final, err := getChecklist(client, srv.URL, checklist.ID)
	if err != nil {
		return fmt.Errorf("get checklist: %w", err)
	}
	if final.Status != domain.StatusComplete {
		return fmt.Errorf("expected status %q after checking all items with no approver, got %q", domain.StatusComplete, final.Status)
	}
	for _, item := range final.Items {
		if !item.Checked {
			return fmt.Errorf("item %d not checked", item.ID)
		}
	}

	return nil
}

func login(client *http.Client, baseURL, username, password string) error {
	form := url.Values{"username": {username}, "password": {password}}
	resp, err := client.PostForm(baseURL+"/login", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// csrfToken reads the checklists_csrf cookie the login response set, for use
// on the X-CSRF-Token header of every subsequent mutating request.
func csrfToken(client *http.Client, baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == "checklists_csrf" {
			return c.Value, nil
		}
	}
	return "", fmt.Errorf("no checklists_csrf cookie set")
}

func doJSON(client *http.Client, method, baseURL, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		token, err := csrfToken(client, baseURL)
		if err != nil {
			return err
		}
		req.Header.Set("X-CSRF-Token", token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, respBody)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func createChecklist(client *http.Client, baseURL string, assignedUserID int64) (*domain.Checklist, error) {
	req := map[string]any{
		"assigned_user_id": assignedUserID,
		"items": []map[string]string{
			{"name": "Smoke test item 1"},
			{"name": "Smoke test item 2"},
		},
	}
	var out domain.Checklist
	if err := doJSON(client, http.MethodPost, baseURL, "/checklists", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func getChecklist(client *http.Client, baseURL string, id int64) (*domain.Checklist, error) {
	var out domain.Checklist
	if err := doJSON(client, http.MethodGet, baseURL, fmt.Sprintf("/checklists/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func checkItem(client *http.Client, baseURL string, checklistID, itemID int64) error {
	path := fmt.Sprintf("/checklists/%d/items/%d/check", checklistID, itemID)
	return doJSON(client, http.MethodPost, baseURL, path, nil, nil)
}
