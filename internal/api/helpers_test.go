//go:build integration

package api_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
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

// newTestServer builds the full mux against testStore and wraps it in an
// httptest.Server.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := api.NewMux(testStore)
	srv := httptest.NewServer(mux)
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
