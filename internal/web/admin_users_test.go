//go:build integration

package web_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCreateUserAsAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	username := uniqueName(t, "newuser")
	form := url.Values{
		"username": {username},
		"name":     {"New User"},
		"password": {"hunter22"},
	}
	resp := doForm(t, client, http.MethodPost, srv.URL+"/admin/users", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), username) {
		t.Errorf("users table missing %q:\n%s", username, body)
	}
}

func TestCreateUserAsNonAdminForbidden(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	form := url.Values{"username": {"whoever"}, "name": {"Whoever"}, "password": {"hunter22"}}
	resp := doForm(t, client, http.MethodPost, srv.URL+"/admin/users", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestBulkCreateUsersCSV(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	u1 := uniqueName(t, "bulk1")
	u2 := uniqueName(t, "bulk2")
	csv := u1 + ",hunter22,Bulk One\n" + u2 + ",hunter22,Bulk Two,true\n"

	uploadURL := srv.URL + "/admin/users/bulk"
	req, err := http.NewRequest(http.MethodPost, uploadURL, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "text/csv")
	if token := csrfTokenFromJar(t, client, uploadURL); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("bulk upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), u1) || !strings.Contains(string(body), u2) {
		t.Errorf("bulk result missing created usernames:\n%s", body)
	}
	if !strings.Contains(string(body), "created") {
		t.Errorf("bulk result missing 'created' status:\n%s", body)
	}
}

func TestAdminUsersPageRendersUserList(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	resp, err := client.Get(srv.URL + "/admin/users")
	if err != nil {
		t.Fatalf("get /admin/users: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), admin.Username) {
		t.Errorf("admin users page missing %q:\n%s", admin.Username, body)
	}
}
