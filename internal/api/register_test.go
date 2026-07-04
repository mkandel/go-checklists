//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRegister_CreatesUserAndLogsIn(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "newuser")

	body := map[string]any{
		"username": username,
		"password": "hunter2hunter2",
		"name":     "New User",
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/register", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		Username string
		IsAdmin  bool
		IsActive bool
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Username != username || created.IsAdmin || !created.IsActive {
		t.Fatalf("created user = %+v, want username=%s is_admin=false is_active=true", created, username)
	}

	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "checklists_session" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected registration to set a checklists_session cookie (auto-login)")
	}

	meResp, err := client.Get(srv.URL + "/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me status = %d, want 200 (expected to be logged in after registering)", meResp.StatusCode)
	}
}

func TestRegister_DuplicateUsername_409(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "taken")
	mustCreateUser(t, username, "hunter2hunter2", true)

	body := map[string]any{"username": username, "password": "hunter2hunter2", "name": "Someone Else"}
	resp := doJSON(t, newClient(t), http.MethodPost, srv.URL+"/register", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestRegister_ShortPassword_400(t *testing.T) {
	srv := newTestServer(t)
	body := map[string]any{"username": uniqueName(t, "shortpw"), "password": "short", "name": "Short Password"}
	resp := doJSON(t, newClient(t), http.MethodPost, srv.URL+"/register", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRegister_MissingUsername_400(t *testing.T) {
	srv := newTestServer(t)
	body := map[string]any{"password": "hunter2hunter2", "name": "No Username"}
	resp := doJSON(t, newClient(t), http.MethodPost, srv.URL+"/register", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRegister_WithEmail_Stored(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "emailed")

	body := map[string]any{
		"username": username,
		"password": "hunter2hunter2",
		"name":     "Emailed User",
		"email":    "emailed@example.com",
	}
	resp := doJSON(t, newClient(t), http.MethodPost, srv.URL+"/register", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Email != "emailed@example.com" {
		t.Fatalf("created.Email = %q, want %q", created.Email, "emailed@example.com")
	}
}
