//go:build integration

package api_test

import (
	"net/http"
	"testing"
)

func TestLoginRateLimit_TripsAfterRepeatedFailures(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)

	// maxLoginAttempts is 5 (internal/api/handlers.go) — exhaust it with
	// wrong-password attempts, all ordinary 401s.
	for i := 0; i < 5; i++ {
		resp := doLogin(t, client, srv.URL, username, "wrong-password")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, resp.StatusCode)
		}
	}

	// The next attempt is blocked by the rate limiter before credentials are
	// even checked — even with the correct password.
	resp := doLogin(t, client, srv.URL, username, "hunter2")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 after exhausting attempts", resp.StatusCode)
	}
}

func TestLoginRateLimit_SuccessClearsCounter(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)

	for i := 0; i < 3; i++ {
		resp := doLogin(t, client, srv.URL, username, "wrong-password")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, resp.StatusCode)
		}
	}

	okResp := doLogin(t, client, srv.URL, username, "hunter2")
	okResp.Body.Close()
	if okResp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d, want 204", okResp.StatusCode)
	}

	// A successful login clears the window, so a fresh run of failures
	// starts from zero rather than continuing the old count towards 429.
	for i := 0; i < 5; i++ {
		resp := doLogin(t, client, srv.URL, username, "wrong-password")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("post-reset attempt %d: status = %d, want 401", i, resp.StatusCode)
		}
	}
}
