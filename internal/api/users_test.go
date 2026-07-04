package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func TestListUsers(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	alice := mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	resp := doJSON(t, client, http.MethodGet, srv.URL+"/users", nil)
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
	resp := doJSON(t, newClient(t), http.MethodGet, srv.URL+"/users", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
