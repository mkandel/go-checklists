// Package api provides the HTTP layer for Checklists.
package api

import (
	"encoding/json"
	"fmt"
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

// minRegisterPasswordLength is the minimum password length accepted by
// handleRegister — this endpoint is reachable by anyone with no prior
// authentication (unlike admin-created users), so it's worth a floor beyond
// "non-empty".
const minRegisterPasswordLength = 8

// NewMux builds the top-level HTTP router serving the JSON API alone. It is
// still used by internal/api's own tests and cmd/smoketest; production
// wiring in cmd/checklists-server/main.go instead calls RegisterRoutes,
// RegisterAuthRoutes, and WithSession directly to build the API and web
// muxes separately.
func NewMux(store *postgres.Store) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux, store)
	return WithSession(store, mux)
}

// RegisterAuthRoutes wires the shared auth endpoints (/login, /register,
// /logout) onto mux. These intentionally keep unprefixed paths — they don't
// collide with any UI page route, and withSession's CSRF exemption checks
// for these exact literal paths. Exported so cmd/checklists-server/main.go
// can register them on the web-facing mux too, without internal/web needing
// to import internal/api.
func RegisterAuthRoutes(mux *http.ServeMux, store *postgres.Store) {
	users, sessions, tenants := store.Users(), store.Sessions(), store.Tenants()
	limiter := auth.NewLoginLimiter(maxLoginAttempts, loginAttemptWindow)

	mux.HandleFunc("POST /login", handleLogin(users, sessions, tenants, limiter))
	mux.HandleFunc("POST /register", handleRegister(users, sessions, tenants))
	mux.HandleFunc("POST /logout", handleLogout(sessions))
}

// RegisterRoutes wires every JSON API route (including auth) onto mux. Every
// non-auth route is namespaced under /api/* so the UI can own the equivalent
// root paths (e.g. /checklists) as human-facing pages.
func RegisterRoutes(mux *http.ServeMux, store *postgres.Store) {
	RegisterAuthRoutes(mux, store)

	mux.HandleFunc("GET /api/healthz", handleHealth)
	mux.Handle("GET /api/me", RequireAuth(http.HandlerFunc(handleMe)))
	registerChecklistRoutes(mux, store)
	registerNotificationRoutes(mux, store)
	registerUserRoutes(mux, store)
	registerTemplateRoutes(mux, store)
	registerGroupRoutes(mux, store)
	registerTenantMailRoutes(mux, store)
	registerTenantChecklistPolicyRoutes(mux, store)
}

// WithSession wraps next with session/CSRF handling — see withSession for
// details. Exported so cmd/checklists-server/main.go can wrap the API and
// web muxes each in their own session/CSRF handling.
func WithSession(store *postgres.Store, next http.Handler) http.Handler {
	return withSession(store.Users(), store.Sessions(), next)
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

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

// handleRegister is self-service registration: anyone may create an
// ordinary (non-admin) account for themselves in the sole tenant. It mirrors
// handleLogin's session/CSRF cookie setup so a successful registration logs
// the new user straight in, the same way a signup form would on most sites.
func handleRegister(users domain.UserRepo, sessions domain.SessionRepo, tenants domain.TenantRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Name == "" {
			http.Error(w, "username and name are required", http.StatusBadRequest)
			return
		}
		if len(req.Password) < minRegisterPasswordLength {
			http.Error(w, fmt.Sprintf("password must be at least %d characters", minRegisterPasswordLength), http.StatusBadRequest)
			return
		}

		// v1 hardcodes the sole tenant — see domain.TenantRepo.GetSoleTenant
		// for why, and DESIGN.md for the v2 per-request resolution plan.
		tenant, err := tenants.GetSoleTenant(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		u := &domain.User{
			TenantID:     tenant.ID,
			Name:         req.Name,
			Username:     req.Username,
			PasswordHash: hash,
			Email:        emailPtr(req.Email),
			IsActive:     true,
		}
		if err := users.Create(r.Context(), u); err != nil {
			writeDomainError(w, err)
			return
		}

		session, err := auth.Login(r.Context(), users, sessions, tenant.ID, req.Username, req.Password)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		csrfToken, err := newCSRFToken()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		setSessionCookie(w, r, session.Token, session.ExpiresAt)
		setCSRFCookie(w, r, csrfToken, session.ExpiresAt)
		writeJSON(w, http.StatusCreated, u)
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
