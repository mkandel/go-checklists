//go:build integration

package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCSRF_MissingHeaderRejected(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	creator := mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	body := map[string]any{
		"assigned_user_id": creator.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	}
	resp := doJSONNoCSRF(t, client, http.MethodPost, srv.URL+"/api/checklists", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (missing CSRF header)", resp.StatusCode)
	}
}

func TestCSRF_MismatchedHeaderRejected(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	creator := mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	payload, err := json.Marshal(map[string]any{
		"assigned_user_id": creator.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/checklists", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "not-the-right-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (mismatched CSRF header)", resp.StatusCode)
	}
}

func TestCSRF_PasswordResetRequestExempt(t *testing.T) {
	srv := newTestServer(t)
	// No X-CSRF-Token header at all, via a client with no prior session —
	// proves the route is reachable pre-authentication, not just that an
	// existing session's CSRF check is bypassed.
	resp := doPasswordResetRequest(t, newClient(t), srv.URL, uniqueName(t, "nobody"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (password-reset/request should be CSRF-exempt)", resp.StatusCode)
	}
}

func TestCSRF_PasswordResetConfirmExempt(t *testing.T) {
	srv := newTestServer(t)
	body := map[string]any{"token": "not-a-real-token", "password": "newpassword123"}
	resp := doJSONNoCSRF(t, newClient(t), http.MethodPost, srv.URL+"/password-reset/confirm", body)
	defer resp.Body.Close()
	// An invalid token still 400s rather than 403ing on a missing CSRF
	// header — proves the route is CSRF-exempt (a non-exempt route would
	// 403 before ever reaching the handler's token check).
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (password-reset/confirm should be CSRF-exempt)", resp.StatusCode)
	}
}

func TestCSRF_CorrectHeaderSucceeds(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	creator := mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	body := map[string]any{
		"assigned_user_id": creator.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/checklists", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (correct CSRF header via doJSON)", resp.StatusCode)
	}
}
