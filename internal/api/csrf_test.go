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
	resp := doJSONNoCSRF(t, client, http.MethodPost, srv.URL+"/checklists", body)
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
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/checklists", bytes.NewReader(payload))
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

func TestCSRF_CorrectHeaderSucceeds(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	creator := mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	body := map[string]any{
		"assigned_user_id": creator.ID,
		"items":            []map[string]string{{"name": "Step 1"}},
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/checklists", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (correct CSRF header via doJSON)", resp.StatusCode)
	}
}
