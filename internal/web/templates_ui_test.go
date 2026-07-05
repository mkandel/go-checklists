//go:build integration

package web_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateTemplateVersionAsAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	name := uniqueName(t, "Deploy Checklist")
	req := map[string]any{
		"name": name,
		"items": []map[string]string{
			{"name": "Run migrations", "validation_ref": "db"},
			{"name": "Smoke test", "validation_ref": ""},
		},
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/templates", req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201: %s", resp.StatusCode, body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero template id")
	}

	detailResp, err := client.Get(fmt.Sprintf("%s/templates/%d", srv.URL, created.ID))
	if err != nil {
		t.Fatalf("get template detail: %v", err)
	}
	defer detailResp.Body.Close()
	body, _ := io.ReadAll(detailResp.Body)
	if !strings.Contains(string(body), name) || !strings.Contains(string(body), "Run migrations") {
		t.Errorf("template detail page missing expected content:\n%s", body)
	}
}

func TestCreateTemplateVersionAsNonAdminForbidden(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	req := map[string]any{
		"name":  uniqueName(t, "NoAccess"),
		"items": []map[string]string{{"name": "Step", "validation_ref": ""}},
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/templates", req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestTemplatesListPage(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	name := uniqueName(t, "ListedTemplate")
	req := map[string]any{
		"name":  name,
		"items": []map[string]string{{"name": "Step one", "validation_ref": ""}},
	}
	createResp := doJSON(t, client, http.MethodPost, srv.URL+"/templates", req)
	createResp.Body.Close()

	resp, err := client.Get(srv.URL + "/templates")
	if err != nil {
		t.Fatalf("get /templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), name) {
		t.Errorf("templates list page missing %q:\n%s", name, body)
	}
}
