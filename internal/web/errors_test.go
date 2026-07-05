//go:build integration

package web_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/web"
)

func TestUnmatchedRoute_RendersCustom404Page(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/no-such-page")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "404") {
		t.Fatalf("body does not contain status code: %s", body)
	}
	if !strings.Contains(body, `href="/checklists"`) {
		t.Fatalf("body does not contain the shared layout nav: %s", body)
	}
}

func TestWithRecover_RendersCustom500PageOnPanic(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	web.WithRecover(panicking).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "500") {
		t.Fatalf("body does not contain status code: %s", rec.Body.String())
	}
}

func TestWithRecover_PassesThroughNonPanickingResponse(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("hello"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	web.WithRecover(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "hello")
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}
