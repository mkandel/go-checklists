//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
)

// mustCreateUserWithEmail is mustCreateUser plus an email address, needed by
// RequestPasswordReset (a user with no email on file can't receive a reset
// link, so RequestPasswordReset treats it the same as an unknown user).
func mustCreateUserWithEmail(t *testing.T, username, password, email string) *domain.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &domain.User{TenantID: testTenantID, Name: username, Username: username, PasswordHash: hash, IsActive: true, Email: &email}
	if err := testStore.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u
}

func TestPasswordResetRequest_KnownUsername_204(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUserWithEmail(t, username, "hunter2", "alice@example.com")

	resp := doPasswordResetRequest(t, newClient(t), srv.URL, username)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestPasswordResetRequest_UnknownUsername_StillReturns204(t *testing.T) {
	srv := newTestServer(t)
	resp := doPasswordResetRequest(t, newClient(t), srv.URL, uniqueName(t, "nobody"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (enumeration-safe)", resp.StatusCode)
	}
}

func TestPasswordResetRequest_RateLimited(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUserWithEmail(t, username, "hunter2", "alice@example.com")

	// maxPasswordResetAttempts is 5 (internal/api/password_reset.go) —
	// exhaust it, then confirm the next attempt is rate-limited.
	for i := 0; i < 5; i++ {
		resp := doPasswordResetRequest(t, client, srv.URL, username)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("attempt %d: status = %d, want 204", i, resp.StatusCode)
		}
	}

	resp := doPasswordResetRequest(t, client, srv.URL, username)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 after exhausting attempts", resp.StatusCode)
	}
}

func TestPasswordResetConfirm_Success(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUserWithEmail(t, username, "hunter2", "alice@example.com")

	// Log in first so we can prove this session gets invalidated by the
	// reset. Keep its client separate from the one performing the reset.
	oldSessionClient := mustLogin(t, srv, username, "hunter2")

	token, _, err := auth.RequestPasswordReset(context.Background(), testStore.Users(), testStore.PasswordResetTokens(), testTenantID, username)
	if err != nil {
		t.Fatalf("request password reset: %v", err)
	}

	client := newClient(t)
	body := map[string]any{"token": token.Token, "password": "newpassword123"}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/password-reset/confirm", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "checklists_session" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected confirm to set a checklists_session cookie (auto-login)")
	}

	// New password works, old password no longer does.
	okLogin := doLogin(t, newClient(t), srv.URL, username, "newpassword123")
	okLogin.Body.Close()
	if okLogin.StatusCode != http.StatusNoContent {
		t.Fatalf("login with new password status = %d, want 204", okLogin.StatusCode)
	}
	badLogin := doLogin(t, newClient(t), srv.URL, username, "hunter2")
	badLogin.Body.Close()
	if badLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login with old password status = %d, want 401", badLogin.StatusCode)
	}

	// The session that existed before the reset is now invalidated.
	meResp, err := oldSessionClient.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /api/me with old session: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old session /api/me status = %d, want 401 (should be invalidated)", meResp.StatusCode)
	}
}

func TestPasswordResetConfirm_InvalidToken_400(t *testing.T) {
	srv := newTestServer(t)
	body := map[string]any{"token": "not-a-real-token", "password": "newpassword123"}
	resp := doJSON(t, newClient(t), http.MethodPost, srv.URL+"/password-reset/confirm", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPasswordResetConfirm_ShortPassword_400(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUserWithEmail(t, username, "hunter2", "alice@example.com")

	token, _, err := auth.RequestPasswordReset(context.Background(), testStore.Users(), testStore.PasswordResetTokens(), testTenantID, username)
	if err != nil {
		t.Fatalf("request password reset: %v", err)
	}

	body := map[string]any{"token": token.Token, "password": "short"}
	resp := doJSON(t, newClient(t), http.MethodPost, srv.URL+"/password-reset/confirm", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func doPasswordResetRequest(t *testing.T, client *http.Client, srv, username string) *http.Response {
	t.Helper()
	resp, err := client.PostForm(srv+"/password-reset/request", map[string][]string{"username": {username}})
	if err != nil {
		t.Fatalf("POST /password-reset/request: %v", err)
	}
	return resp
}
