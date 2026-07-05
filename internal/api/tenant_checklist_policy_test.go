//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

// resetChecklistCreationPolicy clears testTenantID's checklist-creation
// restriction, so a test that enables it doesn't leak that state into other
// tests sharing the same package-level tenant.
func resetChecklistCreationPolicy(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := testStore.Tenants().UpdateChecklistCreationPolicy(context.Background(), testTenantID, domain.ChecklistCreationPolicy{}); err != nil {
			t.Fatalf("reset checklist creation policy: %v", err)
		}
	})
}

func TestChecklistPolicy_GetDefault(t *testing.T) {
	srv := newTestServer(t)
	resetChecklistCreationPolicy(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	resp := doJSON(t, client, http.MethodGet, srv.URL+"/api/admin/tenant/checklist-policy", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Restrict       bool   `json:"restrict"`
		CreatorGroupID *int64 `json:"creator_group_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Restrict {
		t.Fatal("expected restrict=false by default")
	}
}

func TestChecklistPolicy_UpdateThenGet(t *testing.T) {
	srv := newTestServer(t)
	resetChecklistCreationPolicy(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")
	group := mustCreateGroup(t, uniqueName(t, "creators"))

	body := map[string]any{"restrict": true, "creator_group_id": group.ID}
	resp := doJSON(t, client, http.MethodPut, srv.URL+"/api/admin/tenant/checklist-policy", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", resp.StatusCode)
	}

	getResp := doJSON(t, client, http.MethodGet, srv.URL+"/api/admin/tenant/checklist-policy", nil)
	defer getResp.Body.Close()
	var got struct {
		Restrict       bool   `json:"restrict"`
		CreatorGroupID *int64 `json:"creator_group_id"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Restrict || got.CreatorGroupID == nil || *got.CreatorGroupID != group.ID {
		t.Fatalf("got %+v, want restrict=true creator_group_id=%d", got, group.ID)
	}
}

func TestChecklistPolicy_RestrictWithoutGroup_400(t *testing.T) {
	srv := newTestServer(t)
	resetChecklistCreationPolicy(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	resp := doJSON(t, client, http.MethodPut, srv.URL+"/api/admin/tenant/checklist-policy", map[string]any{"restrict": true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestChecklistPolicy_RequiresAdmin_403(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	resp := doJSON(t, client, http.MethodGet, srv.URL+"/api/admin/tenant/checklist-policy", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestChecklistPolicy_RequiresAuth_401(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, newClient(t), http.MethodGet, srv.URL+"/api/admin/tenant/checklist-policy", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestCreateChecklist_RestrictedTenant covers the restriction's effect on
// checklist creation itself (domain.CanCreateChecklist wired into
// handleCreateChecklist), not just the policy read/write endpoints above.
func TestCreateChecklist_RestrictedTenant(t *testing.T) {
	srv := newTestServer(t)
	resetChecklistCreationPolicy(t)

	memberUsername := uniqueName(t, "member")
	member := mustCreateUser(t, memberUsername, "hunter2", true)
	outsiderUsername := uniqueName(t, "outsider")
	outsider := mustCreateUser(t, outsiderUsername, "hunter2", true)
	group := mustCreateGroup(t, uniqueName(t, "creators"), member.ID)

	if err := testStore.Tenants().UpdateChecklistCreationPolicy(context.Background(), testTenantID, domain.ChecklistCreationPolicy{
		Restrict:       true,
		CreatorGroupID: &group.ID,
	}); err != nil {
		t.Fatalf("enable checklist creation restriction: %v", err)
	}

	outsiderClient := mustLogin(t, srv, outsiderUsername, "hunter2")
	resp := doJSON(t, outsiderClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"assigned_user_id": outsider.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider create status = %d, want 403", resp.StatusCode)
	}

	memberClient := mustLogin(t, srv, memberUsername, "hunter2")
	memberResp := doJSON(t, memberClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"assigned_user_id": member.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	})
	defer memberResp.Body.Close()
	if memberResp.StatusCode != http.StatusCreated {
		t.Fatalf("member create status = %d, want 201", memberResp.StatusCode)
	}

	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	adminClient := mustLogin(t, srv, adminUsername, "hunter2")
	adminResp := doJSON(t, adminClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"assigned_user_id": member.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	})
	defer adminResp.Body.Close()
	if adminResp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create status = %d, want 201", adminResp.StatusCode)
	}
}
