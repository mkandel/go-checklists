//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func TestListUsers(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	alice := mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	resp := doJSON(t, client, http.MethodGet, srv.URL+"/api/users", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var users []domain.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	found := false
	for _, u := range users {
		if u.ID == alice.ID {
			found = true
			if u.PasswordHash != "" {
				t.Fatalf("expected password hash to be omitted from JSON, got %+v", u)
			}
		}
	}
	if !found {
		t.Fatalf("expected alice in user list, got %+v", users)
	}
}

func TestListUsers_RequiresAuth(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, newClient(t), http.MethodGet, srv.URL+"/api/users", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdminCreateUser_Success(t *testing.T) {
	srv := newTestServer(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	newUsername := uniqueName(t, "newhire")
	body := map[string]any{
		"username": newUsername,
		"password": "hunter2hunter2",
		"name":     "New Hire",
		"is_admin": true,
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/admin/users", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created domain.User
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Username != newUsername || !created.IsAdmin || !created.IsActive {
		t.Fatalf("created user = %+v, want username=%s is_admin=true is_active=true", created, newUsername)
	}
	if created.PasswordHash != "" {
		t.Fatalf("expected password hash to be omitted from JSON, got %+v", created)
	}
}

func TestAdminCreateUser_WithEmail_Stored(t *testing.T) {
	srv := newTestServer(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	newUsername := uniqueName(t, "newhire")
	body := map[string]any{
		"username": newUsername,
		"password": "hunter2hunter2",
		"name":     "New Hire",
		"email":    "newhire@example.com",
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/admin/users", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created domain.User
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Email == nil || *created.Email != "newhire@example.com" {
		t.Fatalf("created.Email = %v, want %q", created.Email, "newhire@example.com")
	}
}

func TestAdminCreateUser_DuplicateUsername_409(t *testing.T) {
	srv := newTestServer(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	taken := uniqueName(t, "taken")
	mustCreateUser(t, taken, "hunter2", true)

	body := map[string]any{"username": taken, "password": "hunter2hunter2", "name": "Dup"}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/admin/users", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestAdminCreateUser_RequiresAdmin_403(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	body := map[string]any{"username": uniqueName(t, "newhire"), "password": "hunter2hunter2", "name": "New Hire"}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/admin/users", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminCreateUser_RequiresAuth_401(t *testing.T) {
	srv := newTestServer(t)
	body := map[string]any{"username": uniqueName(t, "newhire"), "password": "hunter2hunter2", "name": "New Hire"}
	resp := doJSON(t, newClient(t), http.MethodPost, srv.URL+"/api/admin/users", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdminBulkCreateUsers_MixedRows(t *testing.T) {
	srv := newTestServer(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	dup := uniqueName(t, "dup")
	mustCreateUser(t, dup, "hunter2", true)

	good1 := uniqueName(t, "bulk1")
	good2 := uniqueName(t, "bulk2")
	csv := good1 + ",hunter2hunter2,Bulk One,true,bulk1@example.com\n" +
		good2 + ",hunter2hunter2,Bulk Two\n" +
		dup + ",hunter2hunter2,Duplicate\n" +
		"onlyusername\n"

	resp := doCSV(t, client, srv.URL+"/api/admin/users/bulk", csv)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var results []struct {
		Row      int          `json:"row"`
		Username string       `json:"username"`
		Status   string       `json:"status"`
		Error    string       `json:"error"`
		User     *domain.User `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4: %+v", len(results), results)
	}
	if results[0].Status != "created" || results[0].User == nil || !results[0].User.IsAdmin {
		t.Fatalf("row 1 = %+v, want created admin user", results[0])
	}
	if results[0].User.Email == nil || *results[0].User.Email != "bulk1@example.com" {
		t.Fatalf("row 1 email = %v, want %q", results[0].User.Email, "bulk1@example.com")
	}
	if results[1].Status != "created" || results[1].User == nil || results[1].User.IsAdmin {
		t.Fatalf("row 2 = %+v, want created non-admin user", results[1])
	}
	if results[2].Status != "error" {
		t.Fatalf("row 3 (duplicate username) = %+v, want error", results[2])
	}
	if results[3].Status != "error" {
		t.Fatalf("row 4 (missing columns) = %+v, want error", results[3])
	}

	if _, err := testStore.Users().GetByUsername(context.Background(), testTenantID, good1); err != nil {
		t.Fatalf("expected %s to have been created: %v", good1, err)
	}
}

func TestAdminBulkCreateUsers_RequiresAdmin_403(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	resp := doCSV(t, client, srv.URL+"/api/admin/users/bulk", uniqueName(t, "bulk")+",hunter2hunter2,Bulk\n")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

// doCSV posts body as a text/csv request, attaching the CSRF header the same
// way doJSON does for JSON requests.
func doCSV(t *testing.T, client *http.Client, reqURL, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "text/csv")
	if token := csrfTokenFromJar(t, client, reqURL); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", reqURL, err)
	}
	return resp
}
