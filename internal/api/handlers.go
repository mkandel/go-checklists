// Package api provides the HTTP layer for Checklists.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
)

// NewMux builds the top-level HTTP router.
func NewMux(users domain.UserRepo, sessions domain.SessionRepo) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("POST /login", handleLogin(users, sessions))
	mux.HandleFunc("POST /logout", handleLogout(sessions))
	mux.Handle("GET /me", RequireAuth(http.HandlerFunc(handleMe)))

	return withSession(users, sessions, mux)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLogin(users domain.UserRepo, sessions domain.SessionRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")

		session, err := auth.Login(r.Context(), users, sessions, username, password)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    session.Token,
			Path:     "/",
			Expires:  session.ExpiresAt,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   isSecureRequest(r),
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleLogout(sessions domain.SessionRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			_ = auth.Logout(r.Context(), sessions, cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
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
