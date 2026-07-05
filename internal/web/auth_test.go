//go:build integration

package web_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLoginPageRenders(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/login")
	if err != nil {
		t.Fatalf("get /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `name="username"`) {
		t.Errorf("login page missing username field:\n%s", body)
	}
}

func TestRegisterPageRenders(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/register")
	if err != nil {
		t.Fatalf("get /register: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// TestProtectedPageRedirectsAnonymous confirms requireAuthPage redirects a
// signed-out browser to /login?next=... rather than returning the plain-body
// 401 api.RequireAuth gives fragment/JSON endpoints.
func TestProtectedPageRedirectsAnonymous(t *testing.T) {
	srv := newTestServer(t)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(srv.URL + "/checklists")
	if err != nil {
		t.Fatalf("get /checklists: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/login?next=") {
		t.Errorf("Location = %q, want /login?next=... prefix", loc)
	}
}

func TestLoginThenAccessProtectedPage(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "user")
	mustCreateUser(t, username, "hunter22", true)

	client := mustLogin(t, srv, username, "hunter22")
	resp, err := client.Get(srv.URL + "/checklists")
	if err != nil {
		t.Fatalf("get /checklists: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
