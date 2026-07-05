//go:build integration

package web_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestUpdateMailConfigAsAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	form := url.Values{
		"host":         {"smtp.example.com"},
		"port":         {"587"},
		"username":     {"smtpuser"},
		"password":     {"smtppass"},
		"from_address": {"noreply@example.com"},
	}
	resp := doForm(t, client, http.MethodPut, srv.URL+"/admin/mail-config", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Saved") {
		t.Errorf("mail config fragment missing save confirmation:\n%s", body)
	}
	if !strings.Contains(string(body), "smtp.example.com") {
		t.Errorf("mail config fragment missing saved host:\n%s", body)
	}
}

func TestUpdateMailConfigMissingFieldsShowsError(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	form := url.Values{"host": {""}, "port": {""}, "username": {""}, "from_address": {""}}
	resp := doForm(t, client, http.MethodPut, srv.URL+"/admin/mail-config", form)
	defer resp.Body.Close()
	// Domain-rule validation failures render inline with HTTP 200 (htmx
	// doesn't swap non-2xx responses by default) — see internal/web's
	// withChecklistMutationUI doc comment for the same convention.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "required") {
		t.Errorf("mail config fragment missing validation error:\n%s", body)
	}
}

func TestUpdateMailConfigAsNonAdminForbidden(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	form := url.Values{"host": {"smtp.example.com"}, "port": {"587"}, "username": {"u"}, "from_address": {"a@b.com"}}
	resp := doForm(t, client, http.MethodPut, srv.URL+"/admin/mail-config", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}
