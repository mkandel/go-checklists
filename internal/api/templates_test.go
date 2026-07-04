package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

type templateResponseBody struct {
	domain.Template
	Items []domain.TemplateItem `json:"items"`
}

func decodeTemplateResponse(t *testing.T, resp *http.Response) templateResponseBody {
	t.Helper()
	defer resp.Body.Close()
	var out templateResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode template: %v", err)
	}
	return out
}

func TestCreateTemplateVersion_AdminOnly(t *testing.T) {
	srv := newTestServer(t)
	nonAdminName := uniqueName(t, "alice")
	mustCreateUser(t, nonAdminName, "hunter2", true)
	nonAdminClient := mustLogin(t, srv, nonAdminName, "hunter2")

	body := map[string]any{
		"name": uniqueName(t, "onboarding"),
		"items": []map[string]string{
			{"name": "Step 1"},
		},
	}

	forbiddenResp := doJSON(t, nonAdminClient, http.MethodPost, srv.URL+"/templates", body)
	defer forbiddenResp.Body.Close()
	if forbiddenResp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin create status = %d, want 403", forbiddenResp.StatusCode)
	}

	adminName := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminName, "hunter2")
	adminClient := mustLogin(t, srv, adminName, "hunter2")

	createResp := doJSON(t, adminClient, http.MethodPost, srv.URL+"/templates", body)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create status = %d, want 201", createResp.StatusCode)
	}
	created := decodeTemplateResponse(t, createResp)
	if created.ID == 0 || len(created.Items) != 1 {
		t.Fatalf("unexpected created template: %+v", created)
	}
}

func TestGetTemplateAndListAndLatest(t *testing.T) {
	srv := newTestServer(t)
	adminName := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminName, "hunter2")
	adminClient := mustLogin(t, srv, adminName, "hunter2")

	name := uniqueName(t, "onboarding")
	createResp := doJSON(t, adminClient, http.MethodPost, srv.URL+"/templates", map[string]any{
		"name":  name,
		"items": []map[string]string{{"name": "Step 1"}, {"name": "Step 2"}},
	})
	created := decodeTemplateResponse(t, createResp)

	otherName := uniqueName(t, "alice")
	mustCreateUser(t, otherName, "hunter2", true)
	client := mustLogin(t, srv, otherName, "hunter2")

	getResp := doJSON(t, client, http.MethodGet, fmt.Sprintf("%s/templates/%d", srv.URL, created.ID), nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}
	got := decodeTemplateResponse(t, getResp)
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %+v", got.Items)
	}

	latestResp := doJSON(t, client, http.MethodGet, fmt.Sprintf("%s/templates/latest/%s", srv.URL, name), nil)
	if latestResp.StatusCode != http.StatusOK {
		t.Fatalf("latest status = %d, want 200", latestResp.StatusCode)
	}
	latest := decodeTemplateResponse(t, latestResp)
	if latest.ID != created.ID {
		t.Fatalf("latest ID = %d, want %d", latest.ID, created.ID)
	}

	listResp := doJSON(t, client, http.MethodGet, srv.URL+"/templates", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	defer listResp.Body.Close()
	var templates []domain.Template
	if err := json.NewDecoder(listResp.Body).Decode(&templates); err != nil {
		t.Fatalf("decode templates: %v", err)
	}
	found := false
	for _, tmpl := range templates {
		if tmpl.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created template in list, got %+v", templates)
	}
}
