//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func decodeChecklists(t *testing.T, resp *http.Response) []domain.Checklist {
	t.Helper()
	defer resp.Body.Close()
	var out []domain.Checklist
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode checklists: %v", err)
	}
	return out
}

func TestListChecklists_OnlyRelevantToCaller(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	outsiderName := uniqueName(t, "outsider")
	mustCreateUser(t, outsiderName, "hunter2", true)

	creatorClient := mustLogin(t, srv, creatorName, "hunter2")
	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"assigned_user_id": creator.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	})
	created := decodeChecklist(t, createResp)

	creatorListResp := doJSON(t, creatorClient, http.MethodGet, srv.URL+"/api/checklists", nil)
	if creatorListResp.StatusCode != http.StatusOK {
		t.Fatalf("creator list status = %d, want 200", creatorListResp.StatusCode)
	}
	creatorList := decodeChecklists(t, creatorListResp)
	found := false
	for _, c := range creatorList {
		if c.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected creator's list to include checklist %d, got %+v", created.ID, creatorList)
	}

	outsiderClient := mustLogin(t, srv, outsiderName, "hunter2")
	outsiderListResp := doJSON(t, outsiderClient, http.MethodGet, srv.URL+"/api/checklists", nil)
	outsiderList := decodeChecklists(t, outsiderListResp)
	for _, c := range outsiderList {
		if c.ID == created.ID {
			t.Fatalf("expected outsider's list not to include checklist %d", created.ID)
		}
	}
}

func TestListChecklists_StatusFilter(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	creatorClient := mustLogin(t, srv, creatorName, "hunter2")

	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"assigned_user_id": creator.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	})
	created := decodeChecklist(t, createResp)

	badResp := doJSON(t, creatorClient, http.MethodGet, srv.URL+"/api/checklists?status=bogus", nil)
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad status filter = %d, want 400", badResp.StatusCode)
	}

	openResp := doJSON(t, creatorClient, http.MethodGet, srv.URL+"/api/checklists?status=open", nil)
	openList := decodeChecklists(t, openResp)
	found := false
	for _, c := range openList {
		if c.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected open checklist in status=open results")
	}

	completeResp := doJSON(t, creatorClient, http.MethodGet, srv.URL+"/api/checklists?status=complete", nil)
	completeList := decodeChecklists(t, completeResp)
	for _, c := range completeList {
		if c.ID == created.ID {
			t.Fatalf("expected open checklist excluded from status=complete results")
		}
	}
}
