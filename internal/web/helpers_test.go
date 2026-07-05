//go:build integration

package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/web"
)

// sessionCookieName and csrfCookieName mirror internal/api's identical
// unexported constants (internal/web_test can't import internal/api's
// unexported names directly, and this package's tests need them to read the
// CSRF token back out of the cookie jar).
const (
	sessionCookieName = "checklists_session"
	csrfCookieName    = "checklists_csrf"
)

// uniqueName derives a value unique to the calling test (via t.Name()) so
// tests sharing one database don't collide on users.username's unique
// constraint.
func uniqueName(t *testing.T, suffix string) string {
	t.Helper()
	return strings.ReplaceAll(t.Name(), "/", "_") + "_" + suffix
}

func mustCreateUser(t *testing.T, username, password string, active bool) *domain.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &domain.User{TenantID: testTenantID, Name: username, Username: username, PasswordHash: hash, IsActive: active}
	if err := testStore.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u
}

func mustCreateAdminUser(t *testing.T, username, password string) *domain.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &domain.User{TenantID: testTenantID, Name: username, Username: username, PasswordHash: hash, IsActive: true, IsAdmin: true}
	if err := testStore.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("create admin user %s: %v", username, err)
	}
	return u
}

func mustCreateGroup(t *testing.T, name string, memberIDs ...int64) *domain.Group {
	t.Helper()
	g := &domain.Group{TenantID: testTenantID, Name: name}
	if err := testStore.Groups().Create(context.Background(), g); err != nil {
		t.Fatalf("create group: %v", err)
	}
	for _, uid := range memberIDs {
		if err := testStore.Groups().AddMember(context.Background(), testTenantID, g.ID, uid); err != nil {
			t.Fatalf("add group member: %v", err)
		}
	}
	return g
}

// newTestServer builds the same combined mux cmd/checklists-server/main.go
// wires in production — both the JSON API (whose /login, /register, /logout
// the UI pages depend on) and the web UI, wrapped exactly once in
// api.WithSession — and returns it as an httptest.Server.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	api.RegisterRoutes(mux, testStore)
	web.RegisterRoutes(mux, testStore)
	handler := api.WithSession(testStore, mux)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// newClient returns an http.Client with a cookie jar, so a Set-Cookie from
// /login automatically flows to subsequent requests against srv, mirroring
// real browser behavior.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("new cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func doLogin(t *testing.T, client *http.Client, srv, username, password string) *http.Response {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	resp, err := client.PostForm(srv+"/login", form)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	return resp
}

func mustLogin(t *testing.T, srv *httptest.Server, username, password string) *http.Client {
	t.Helper()
	client := newClient(t)
	resp := doLogin(t, client, srv.URL, username, password)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login failed: status = %d", resp.StatusCode)
	}
	return client
}

func csrfTokenFromJar(t *testing.T, client *http.Client, reqURL string) string {
	t.Helper()
	u, err := url.Parse(reqURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == csrfCookieName {
			return c.Value
		}
	}
	return ""
}

// doForm sends a form-encoded request, automatically attaching the CSRF
// header from client's cookie jar — the shape every internal/web mutation
// fragment (groups, admin users, mail config, checklist policy, checklist
// detail actions) expects, since they r.ParseForm() rather than decode JSON.
func doForm(t *testing.T, client *http.Client, method, reqURL string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token := csrfTokenFromJar(t, client, reqURL); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// doJSON sends a JSON request, automatically attaching the CSRF header from
// client's cookie jar — used by the handful of internal/web endpoints
// (checklist create, template version create) that decode a JSON body
// instead of a form.
func doJSON(t *testing.T, client *http.Client, method, reqURL string, body any) *http.Response {
	t.Helper()
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = b
	}
	req, err := http.NewRequest(method, reqURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := csrfTokenFromJar(t, client, reqURL); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}
