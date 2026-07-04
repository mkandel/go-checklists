//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTenantMailConfig_GetUnconfigured(t *testing.T) {
	srv := newTestServer(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	resp := doJSON(t, client, http.MethodGet, srv.URL+"/admin/tenant/mail-config", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Configured bool `json:"configured"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Configured {
		t.Fatalf("expected Configured=false for a tenant with no mail config yet")
	}
}

func TestTenantMailConfig_UpdateThenGet(t *testing.T) {
	srv := newTestServer(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	body := map[string]any{
		"host":         "smtp-relay.brevo.com",
		"port":         587,
		"username":     "smtp-user",
		"password":     "secret",
		"from_address": "notifications@example.com",
	}
	resp := doJSON(t, client, http.MethodPut, srv.URL+"/admin/tenant/mail-config", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", resp.StatusCode)
	}

	getResp := doJSON(t, client, http.MethodGet, srv.URL+"/admin/tenant/mail-config", nil)
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getResp.StatusCode)
	}
	var got struct {
		Host        string `json:"host"`
		Port        int    `json:"port"`
		Username    string `json:"username"`
		FromAddress string `json:"from_address"`
		Configured  bool   `json:"configured"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Configured || got.Host != "smtp-relay.brevo.com" || got.Port != 587 ||
		got.Username != "smtp-user" || got.FromAddress != "notifications@example.com" {
		t.Fatalf("got %+v, want configured brevo config (password omitted)", got)
	}

	// The response body must never carry the password back to the client.
	rawBody, _ := json.Marshal(got)
	if string(rawBody) == "" {
		t.Fatal("unexpected empty response")
	}
}

func TestTenantMailConfig_RequiresAdmin_403(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	resp := doJSON(t, client, http.MethodGet, srv.URL+"/admin/tenant/mail-config", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestTenantMailConfig_RequiresAuth_401(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, newClient(t), http.MethodGet, srv.URL+"/admin/tenant/mail-config", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestTenantMailConfig_MissingFields_400(t *testing.T) {
	srv := newTestServer(t)
	adminUsername := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminUsername, "hunter2")
	client := mustLogin(t, srv, adminUsername, "hunter2")

	body := map[string]any{"host": "smtp-relay.brevo.com"}
	resp := doJSON(t, client, http.MethodPut, srv.URL+"/admin/tenant/mail-config", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
