// Package api provides the HTTP layer for Checklists.
package api

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// maxLoginAttempts and loginAttemptWindow bound how many failed logins a
// single client IP may make before handleLogin starts responding 429.
const (
	maxLoginAttempts   = 5
	loginAttemptWindow = 15 * time.Minute
)

// NewMux builds the top-level HTTP router.
func NewMux(store *postgres.Store) http.Handler {
	users, sessions, tenants := store.Users(), store.Sessions(), store.Tenants()
	limiter := auth.NewLoginLimiter(maxLoginAttempts, loginAttemptWindow)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("POST /login", handleLogin(users, sessions, tenants, limiter))
	mux.HandleFunc("POST /logout", handleLogout(sessions))
	mux.Handle("GET /me", RequireAuth(http.HandlerFunc(handleMe)))
	registerChecklistRoutes(mux, store)
	registerNotificationRoutes(mux, store)
	registerUserRoutes(mux, store)
	registerTemplateRoutes(mux, store)
	registerGroupRoutes(mux, store)

	return withSession(users, sessions, mux)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// loginRateLimitKey derives the LoginLimiter key for r — the client's IP,
// falling back to the raw RemoteAddr if it isn't a host:port pair (rather
// than failing open and skipping the rate limit entirely).
func loginRateLimitKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func handleLogin(users domain.UserRepo, sessions domain.SessionRepo, tenants domain.TenantRepo, limiter *auth.LoginLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := loginRateLimitKey(r)
		if !limiter.Allow(key) {
			http.Error(w, "too many login attempts, try again later", http.StatusTooManyRequests)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")

		// v1 hardcodes the sole tenant — see domain.TenantRepo.GetSoleTenant
		// for why, and DESIGN.md for the v2 per-request resolution plan.
		tenant, err := tenants.GetSoleTenant(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		session, err := auth.Login(r.Context(), users, sessions, tenant.ID, username, password)
		if err != nil {
			limiter.RecordFailure(key)
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		limiter.RecordSuccess(key)

		csrfToken, err := newCSRFToken()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		setSessionCookie(w, r, session.Token, session.ExpiresAt)
		setCSRFCookie(w, r, csrfToken, session.ExpiresAt)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleLogout(sessions domain.SessionRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			_ = auth.Logout(r.Context(), sessions, cookie.Value)
		}
		clearCookie(w, sessionCookieName)
		clearCookie(w, csrfCookieName)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context()) // RequireAuth guarantees presence
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u)
}

// isSecureRequest reports whether the cookie's Secure flag should be set —
// true unless the request is plain-http (no TLS termination configured yet).
func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil
}
