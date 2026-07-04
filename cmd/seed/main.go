// Command seed populates a sample dataset for local development: a default
// tenant, a handful of users (including an admin), a group, a template, and
// checklists spanning every lifecycle status. It's idempotent — re-running
// it against an already-seeded database skips whatever already exists rather
// than erroring or duplicating rows.
package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/config"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// samplePassword is used for every seeded user. It's dev-only data seeded
// into a local/sample database, never production.
const samplePassword = "password123"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cfg, err := config.Load(os.Args[1:], os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

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

	tenant, err := ensureDefaultTenant(ctx, store)
	if err != nil {
		log.Fatalf("ensure default tenant: %v", err)
	}
	log.Printf("tenant: %s (id=%d)", tenant.Slug, tenant.ID)

	admin := mustUser(ctx, store, tenant.ID, "admin", "Admin", true)
	alice := mustUser(ctx, store, tenant.ID, "alice", "Alice", false)
	bob := mustUser(ctx, store, tenant.ID, "bob", "Bob", false)
	carol := mustUser(ctx, store, tenant.ID, "carol", "Carol", false)

	group := mustGroup(ctx, store, tenant.ID, "Ops Team", []*domain.User{alice, bob})

	template := mustTemplate(ctx, store, tenant.ID, "Server Deployment", []domain.TemplateItem{
		{Name: "Back up database", ValidationRef: ""},
		{Name: "Deploy new build", ValidationRef: ""},
		{Name: "Run smoke tests", ValidationRef: ""},
		{Name: "Notify stakeholders", ValidationRef: ""},
	})

	seedChecklists(ctx, store, tenant.ID, admin, alice, bob, carol, group, template)

	log.Print("seed complete")
}

// ensureDefaultTenant mirrors cmd/checklists-server's provisioning so the
// sample database looks like a freshly-started on-prem install.
func ensureDefaultTenant(ctx context.Context, store *postgres.Store) (*domain.Tenant, error) {
	if t, err := store.Tenants().GetSoleTenant(ctx); err == nil {
		return t, nil
	}
	t := &domain.Tenant{Name: "Default", Slug: "default"}
	if err := store.Tenants().Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func mustUser(ctx context.Context, store *postgres.Store, tenantID int64, username, name string, isAdmin bool) *domain.User {
	if u, err := store.Users().GetByUsername(ctx, tenantID, username); err == nil {
		return u
	}
	hash, err := auth.HashPassword(samplePassword)
	if err != nil {
		log.Fatalf("hash password for %s: %v", username, err)
	}
	u := &domain.User{
		TenantID:     tenantID,
		Name:         name,
		Username:     username,
		PasswordHash: hash,
		IsActive:     true,
		IsAdmin:      isAdmin,
	}
	if err := store.Users().Create(ctx, u); err != nil {
		log.Fatalf("create user %s: %v", username, err)
	}
	log.Printf("user: %s (id=%d, admin=%v)", username, u.ID, isAdmin)
	return u
}

func mustGroup(ctx context.Context, store *postgres.Store, tenantID int64, name string, members []*domain.User) *domain.Group {
	groups, err := store.Groups().List(ctx, tenantID)
	if err != nil {
		log.Fatalf("list groups: %v", err)
	}
	for _, g := range groups {
		if g.Name == name {
			return &g
		}
	}

	g := &domain.Group{TenantID: tenantID, Name: name}
	if err := store.Groups().Create(ctx, g); err != nil {
		log.Fatalf("create group %s: %v", name, err)
	}
	for _, m := range members {
		if err := store.Groups().AddMember(ctx, tenantID, g.ID, m.ID); err != nil {
			log.Fatalf("add %s to group %s: %v", m.Username, name, err)
		}
	}
	log.Printf("group: %s (id=%d)", name, g.ID)
	return g
}

func mustTemplate(ctx context.Context, store *postgres.Store, tenantID int64, name string, items []domain.TemplateItem) *domain.Template {
	if t, _, err := store.Templates().GetLatestByName(ctx, tenantID, name); err == nil {
		return t
	}
	t := &domain.Template{TenantID: tenantID, Name: name, Version: 1}
	if err := store.Templates().CreateVersion(ctx, t, items); err != nil {
		log.Fatalf("create template %s: %v", name, err)
	}
	log.Printf("template: %s v%d (id=%d)", name, t.Version, t.ID)
	return t
}

// seedChecklists creates one checklist per lifecycle status, skipping the
// whole step if admin has already created checklists in tenantID (the
// idempotency check for this function).
func seedChecklists(ctx context.Context, store *postgres.Store, tenantID int64, admin, alice, bob, carol *domain.User, group *domain.Group, template *domain.Template) {
	existing, err := store.Checklists().List(ctx, domain.ChecklistFilter{TenantID: tenantID, UserID: admin.ID})
	if err != nil {
		log.Fatalf("list checklists: %v", err)
	}
	if len(existing) > 0 {
		log.Printf("checklists: %d already exist for admin, skipping", len(existing))
		return
	}

	// 1. Ad-hoc checklist, open, directly assigned to alice.
	adhoc := &domain.Checklist{
		TenantID:       tenantID,
		CreatorID:      admin.ID,
		AssignedUserID: &alice.ID,
		Status:         domain.StatusOpen,
		Items: []domain.ChecklistItem{
			{Name: "Water the office plants"},
			{Name: "Restock coffee"},
		},
	}
	mustCreateChecklist(ctx, store, adhoc, "ad-hoc, open, assigned to alice")

	// 2. From template, open, assigned to the group (unclaimed).
	unclaimed := &domain.Checklist{
		TenantID:        tenantID,
		TemplateID:      &template.ID,
		CreatorID:       admin.ID,
		AssignedGroupID: &group.ID,
		ApproverID:      &carol.ID,
		Status:          domain.StatusOpen,
	}
	mustCreateChecklist(ctx, store, unclaimed, "from template, open, unclaimed, assigned to Ops Team")

	// 3. From template, claimed by bob, first item checked.
	inProgress := &domain.Checklist{
		TenantID:        tenantID,
		TemplateID:      &template.ID,
		CreatorID:       admin.ID,
		AssignedGroupID: &group.ID,
		AssignedUserID:  &bob.ID,
		ApproverID:      &carol.ID,
		Status:          domain.StatusOpen,
	}
	mustCreateChecklist(ctx, store, inProgress, "from template, claimed by bob, in progress")
	inProgress.Items[0].Checked = true
	inProgress.Items[0].CheckedBy = &bob.ID
	if err := store.Checklists().Save(ctx, inProgress, []domain.Event{{
		TenantID:    tenantID,
		ChecklistID: inProgress.ID,
		ItemID:      &inProgress.Items[0].ID,
		ActorUserID: bob.ID,
		Action:      domain.EventItemChecked,
	}}); err != nil {
		log.Fatalf("check first item on in-progress checklist: %v", err)
	}

	// 4. From template, every item checked, submitted for validation.
	validating := &domain.Checklist{
		TenantID:       tenantID,
		TemplateID:     &template.ID,
		CreatorID:      admin.ID,
		AssignedUserID: &bob.ID,
		ApproverID:     &carol.ID,
		Status:         domain.StatusOpen,
	}
	mustCreateChecklist(ctx, store, validating, "from template, awaiting approval")
	for i := range validating.Items {
		validating.Items[i].Checked = true
		validating.Items[i].CheckedBy = &bob.ID
	}
	validating.Status = domain.StatusValidating
	if err := store.Checklists().Save(ctx, validating, []domain.Event{{
		TenantID:    tenantID,
		ChecklistID: validating.ID,
		ActorUserID: bob.ID,
		Action:      domain.EventSubmittedForValidation,
	}}); err != nil {
		log.Fatalf("submit checklist for validation: %v", err)
	}

	// 5. From template, approved and complete.
	complete := &domain.Checklist{
		TenantID:       tenantID,
		TemplateID:     &template.ID,
		CreatorID:      admin.ID,
		AssignedUserID: &alice.ID,
		ApproverID:     &carol.ID,
		Status:         domain.StatusOpen,
	}
	mustCreateChecklist(ctx, store, complete, "from template, complete")
	for i := range complete.Items {
		complete.Items[i].Checked = true
		complete.Items[i].CheckedBy = &alice.ID
	}
	complete.Status = domain.StatusComplete
	if err := store.Checklists().Save(ctx, complete, []domain.Event{
		{TenantID: tenantID, ChecklistID: complete.ID, ActorUserID: alice.ID, Action: domain.EventSubmittedForValidation},
		{TenantID: tenantID, ChecklistID: complete.ID, ActorUserID: carol.ID, Action: domain.EventApproved},
		{TenantID: tenantID, ChecklistID: complete.ID, ActorUserID: carol.ID, Action: domain.EventCompleted},
	}); err != nil {
		log.Fatalf("complete checklist: %v", err)
	}
}

func mustCreateChecklist(ctx context.Context, store *postgres.Store, c *domain.Checklist, label string) {
	if err := store.Checklists().Create(ctx, c); err != nil {
		log.Fatalf("create checklist (%s): %v", label, err)
	}
	log.Printf("checklist: %s (id=%d)", label, c.ID)
}
