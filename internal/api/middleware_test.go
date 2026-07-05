//go:build integration

package api_test

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/api"
)

func TestWithAccessLog_PassesThroughResponse(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("hello"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	api.WithAccessLog(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "hello")
	}
}

func TestWithAccessLog_LogsMethodPathAndStatus(t *testing.T) {
	var buf bytes.Buffer
	prevOutput, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/checklists", nil)
	req.RemoteAddr = "203.0.113.5:54321"
	api.WithAccessLog(inner).ServeHTTP(rec, req)

	got := buf.String()
	for _, want := range []string{"203.0.113.5", "POST", "/api/checklists", "201"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log line %q does not contain %q", got, want)
		}
	}
}

func TestWithAccessLog_DefaultsToOKWhenHandlerNeverWritesHeader(t *testing.T) {
	var buf bytes.Buffer
	prevOutput, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("no explicit status"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	api.WithAccessLog(inner).ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "200") {
		t.Fatalf("log line %q does not contain default status 200", buf.String())
	}
}
