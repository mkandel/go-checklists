//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func doLogin(t *testing.T, client *http.Client, srv string, username, password string) *http.Response {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	resp, err := client.PostForm(srv+"/login", form)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	return resp
}

func TestLogin_Success_SetsCookie(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)

	resp := doLogin(t, client, srv.URL, username, "hunter2")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "checklists_session" {
			found = true
			if !c.HttpOnly {
				t.Fatal("expected session cookie to be HttpOnly")
			}
		}
	}
	if !found {
		t.Fatal("expected a checklists_session cookie to be set")
	}
}

func TestLogin_WrongPassword_401(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)

	resp := doLogin(t, client, srv.URL, username, "wrong")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestLogin_UnknownUser_401(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)

	resp := doLogin(t, client, srv.URL, uniqueName(t, "nobody"), "whatever")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestLogin_InactiveUser_401(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", false)

	resp := doLogin(t, client, srv.URL, username, "hunter2")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestMe_Unauthenticated_401(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)

	resp, err := client.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestMe_Authenticated_ReturnsUser(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)

	loginResp := doLogin(t, client, srv.URL, username, "hunter2")
	loginResp.Body.Close()

	resp, err := client.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got struct {
		Username string `json:"Username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode /me body: %v", err)
	}
	if got.Username != username {
		t.Fatalf("Username = %q, want %q", got.Username, username)
	}
}

func TestLogout_ClearsCookieAndInvalidatesSession(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)

	loginResp := doLogin(t, client, srv.URL, username, "hunter2")
	loginResp.Body.Close()

	logoutResp := doJSON(t, client, http.MethodPost, srv.URL+"/logout", nil)
	logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", logoutResp.StatusCode)
	}

	resp, err := client.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /me after logout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status after logout = %d, want 401", resp.StatusCode)
	}
}
