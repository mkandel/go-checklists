// Command stresstest fires N concurrent claim requests at a single
// checklist against a real Postgres database, to verify the FOR UPDATE row
// lock in ChecklistRepo.Claim actually serializes concurrent claims under
// real goroutine contention — something the existing sequential
// TestClaimChecklist_HappyPathAndLostRace / TestChecklistRepo_ClaimRace
// tests can't exercise, since they call Claim one goroutine at a time.
// Reports PASS/FAIL via log output and exit code.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/config"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

const stressPassword = "stresstest-password"

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	n := flag.Int("n", 50, "number of concurrent claimants")
	flag.Parse()

	if err := run(*n); err != nil {
		log.Fatalf("STRESS TEST FAILED: %v", err)
	}
	log.Print("STRESS TEST PASSED")
}

func run(n int) error {
	if n < 2 {
		return fmt.Errorf("n must be at least 2, got %d", n)
	}

	ctx := context.Background()

	cfg, err := config.Load(nil, os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

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

	log.Printf("creating %d claimant users + a shared group...", n)
	users := make([]*domain.User, n)
	memberIDs := make([]int64, n)
	hash, err := auth.HashPassword(stressPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	suffix := time.Now().UnixNano()
	for i := range users {
		u := &domain.User{
			TenantID:     tenant.ID,
			Name:         fmt.Sprintf("Stress Claimant %d", i),
			Username:     fmt.Sprintf("stresstest-%d-%d", suffix, i),
			PasswordHash: hash,
			IsActive:     true,
		}
		if err := store.Users().Create(ctx, u); err != nil {
			return fmt.Errorf("create claimant %d: %w", i, err)
		}
		users[i] = u
		memberIDs[i] = u.ID
	}

	group := &domain.Group{TenantID: tenant.ID, Name: fmt.Sprintf("Stress Group %d", suffix)}
	if err := store.Groups().Create(ctx, group); err != nil {
		return fmt.Errorf("create group: %w", err)
	}
	for _, id := range memberIDs {
		if err := store.Groups().AddMember(ctx, tenant.ID, group.ID, id); err != nil {
			return fmt.Errorf("add group member %d: %w", id, err)
		}
	}

	template := &domain.Template{TenantID: tenant.ID, Name: fmt.Sprintf("Stress Template %d", suffix)}
	if err := store.Templates().CreateVersion(ctx, template, []domain.TemplateItem{{Name: "Stress test item"}}); err != nil {
		return fmt.Errorf("create template: %w", err)
	}

	checklist := &domain.Checklist{
		TenantID:        tenant.ID,
		TemplateID:      template.ID,
		CreatorID:       users[0].ID,
		AssignedGroupID: &group.ID,
	}
	if err := store.Checklists().Create(ctx, checklist); err != nil {
		return fmt.Errorf("create checklist: %w", err)
	}

	srv := httptest.NewServer(api.NewMux(store))
	defer srv.Close()

	claimURL := fmt.Sprintf("%s/api/checklists/%d/claim", srv.URL, checklist.ID)

	log.Printf("firing %d concurrent claim requests...", n)
	var (
		wg           sync.WaitGroup
		successCount int64
		conflictCode int64
		otherErrs    int64
	)
	start := make(chan struct{})
	for i, u := range users {
		wg.Add(1)
		go func(i int, u *domain.User) {
			defer wg.Done()

			jar, err := cookiejar.New(nil)
			if err != nil {
				atomic.AddInt64(&otherErrs, 1)
				log.Printf("claimant %d: cookie jar: %v", i, err)
				return
			}
			client := &http.Client{Jar: jar}
			if err := login(client, srv.URL, u.Username, stressPassword); err != nil {
				atomic.AddInt64(&otherErrs, 1)
				log.Printf("claimant %d: login: %v", i, err)
				return
			}
			token, err := csrfToken(client, srv.URL)
			if err != nil {
				atomic.AddInt64(&otherErrs, 1)
				log.Printf("claimant %d: csrf token: %v", i, err)
				return
			}

			<-start // release every goroutine at (as close to) the same instant as possible

			req, err := http.NewRequest(http.MethodPost, claimURL, nil)
			if err != nil {
				atomic.AddInt64(&otherErrs, 1)
				return
			}
			req.Header.Set("X-CSRF-Token", token)
			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&otherErrs, 1)
				log.Printf("claimant %d: claim request: %v", i, err)
				return
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)

			switch resp.StatusCode {
			case http.StatusNoContent:
				atomic.AddInt64(&successCount, 1)
			case http.StatusConflict:
				atomic.AddInt64(&conflictCode, 1)
			default:
				atomic.AddInt64(&otherErrs, 1)
				log.Printf("claimant %d: unexpected status %d", i, resp.StatusCode)
			}
		}(i, u)
	}
	close(start)
	wg.Wait()

	log.Printf("results: %d succeeded, %d conflicted, %d unexpected errors", successCount, conflictCode, otherErrs)

	if otherErrs != 0 {
		return fmt.Errorf("%d claimants hit an unexpected error (see log above)", otherErrs)
	}
	if successCount != 1 {
		return fmt.Errorf("expected exactly 1 successful claim, got %d (row lock did not serialize claims)", successCount)
	}
	if conflictCode != int64(n-1) {
		return fmt.Errorf("expected %d conflicts, got %d", n-1, conflictCode)
	}

	got, err := store.Checklists().Get(ctx, tenant.ID, checklist.ID)
	if err != nil {
		return fmt.Errorf("get checklist: %w", err)
	}
	if got.AssignedUserID == nil {
		return fmt.Errorf("checklist has no assigned user after claiming")
	}

	for _, u := range users {
		if u.ID == *got.AssignedUserID {
			continue
		}
		notifications, err := store.Notifications().ListForUser(ctx, tenant.ID, u.ID)
		if err != nil {
			return fmt.Errorf("list notifications for loser %d: %w", u.ID, err)
		}
		if len(notifications) == 0 {
			return fmt.Errorf("loser %d got no claim_lost notification", u.ID)
		}
	}

	log.Printf("checklist %d correctly claimed by exactly one of %d concurrent claimants", checklist.ID, n)
	return nil
}

func login(client *http.Client, baseURL, username, password string) error {
	form := url.Values{"username": {username}, "password": {password}}
	resp, err := client.PostForm(baseURL+"/login", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

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
