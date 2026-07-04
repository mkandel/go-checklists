package postgres_test

import (
	"context"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func TestTemplateRepo_CreateAndFetchRoundTrip(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t, "onboarding")

	tmpl := &domain.Template{TenantID: testTenantID, Name: name}
	items := []domain.TemplateItem{
		{Name: "Set up laptop"},
		{Name: "Sign paperwork"},
		{Name: "Meet the team"},
	}
	if err := testStore.Templates().CreateVersion(ctx, tmpl, items); err != nil {
		t.Fatalf("create template version: %v", err)
	}
	if tmpl.Version != 1 {
		t.Fatalf("expected version 1, got %d", tmpl.Version)
	}

	got, gotItems, err := testStore.Templates().GetLatestByName(ctx, testTenantID, name)
	if err != nil {
		t.Fatalf("get latest by name: %v", err)
	}
	if got.ID != tmpl.ID || got.Version != 1 {
		t.Fatalf("unexpected template: %+v", got)
	}
	if len(gotItems) != 3 {
		t.Fatalf("expected 3 items, got %d", len(gotItems))
	}
	wantOrder := []string{"Set up laptop", "Sign paperwork", "Meet the team"}
	for i, it := range gotItems {
		if it.Name != wantOrder[i] {
			t.Fatalf("item %d: expected %q, got %q", i, wantOrder[i], it.Name)
		}
		if it.Position != i {
			t.Fatalf("item %d: expected position %d, got %d", i, i, it.Position)
		}
	}

	// A second CreateVersion for the same name bumps the version rather
	// than mutating the first one.
	tmpl2 := &domain.Template{TenantID: testTenantID, Name: name}
	if err := testStore.Templates().CreateVersion(ctx, tmpl2, []domain.TemplateItem{{Name: "Revised step"}}); err != nil {
		t.Fatalf("create template version 2: %v", err)
	}
	if tmpl2.Version != 2 {
		t.Fatalf("expected version 2, got %d", tmpl2.Version)
	}

	latest, _, err := testStore.Templates().GetLatestByName(ctx, testTenantID, name)
	if err != nil {
		t.Fatalf("get latest by name after v2: %v", err)
	}
	if latest.Version != 2 {
		t.Fatalf("expected latest version 2, got %d", latest.Version)
	}

	original, originalItems, err := testStore.Templates().Get(ctx, testTenantID, tmpl.ID)
	if err != nil {
		t.Fatalf("get original version by id: %v", err)
	}
	if original.Version != 1 || len(originalItems) != 3 {
		t.Fatalf("expected original version 1 with 3 items untouched, got version=%d items=%d", original.Version, len(originalItems))
	}
}
