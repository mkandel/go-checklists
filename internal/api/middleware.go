package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
)

const (
	sessionCookieName = "checklists_session"
	csrfCookieName    = "checklists_csrf"
	csrfHeaderName    = "X-CSRF-Token"
)

type contextKey int

const userContextKey contextKey = 0

// UserFromContext returns the authenticated user attached to ctx by
// withSession, if any.
func UserFromContext(ctx context.Context) (*domain.User, bool) {
	u, ok := ctx.Value(userContextKey).(*domain.User)
	return u, ok
}

// newCSRFToken generates a random, URL-safe CSRF token, the same shape as a
// session token.
func newCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// isSafeMethod reports whether method never mutates state, and so is exempt
// from CSRF checks.
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// withSession wraps next, resolving the session cookie (if present) into a
// *domain.User and attaching it to the request context. It never rejects a
// request on its own for lack of a session — use RequireAuth to enforce
// authentication on specific routes. It DOES reject non-safe-method requests
// that carry a resolved session but fail the CSRF double-submit check, since
// every mutating route in this API requires auth and this is the one place
// every request passes through.
func withSession(users domain.UserRepo, sessions domain.SessionRepo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		u, s, renewed, err := auth.CurrentUser(r.Context(), users, sessions, cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		// /login and /register are exempt even when a session cookie happens
		// to be present (e.g. a client re-authenticating without having
		// logged out first) — they prove identity via credentials (or create
		// a fresh identity), not the session they're about to replace.
		if !isSafeMethod(r.Method) && r.URL.Path != "/login" && r.URL.Path != "/register" {
			csrfCookie, err := r.Cookie(csrfCookieName)
			header := r.Header.Get(csrfHeaderName)
			if err != nil || header == "" ||
				subtle.ConstantTimeCompare([]byte(csrfCookie.Value), []byte(header)) != 1 {
				http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
				return
			}
		}

		if renewed {
			setSessionCookie(w, r, s.Token, s.ExpiresAt)
			csrfValue, err := currentOrNewCSRFValue(r)
			if err == nil {
				setCSRFCookie(w, r, csrfValue, s.ExpiresAt)
			}
		}

		ctx := context.WithValue(r.Context(), userContextKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// currentOrNewCSRFValue returns the CSRF cookie value already on r, or a
// freshly generated one if r has none (e.g. an old session predating CSRF
// cookies, or a client that cleared its cookies without logging out).
func currentOrNewCSRFValue(r *http.Request) (string, error) {
	if cookie, err := r.Cookie(csrfCookieName); err == nil {
		return cookie.Value, nil
	}
	return newCSRFToken()
}

// setSessionCookie sets the session cookie carrying token, expiring at
// expiresAt.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

// setCSRFCookie sets the CSRF cookie carrying value, expiring at expiresAt.
// Deliberately NOT HttpOnly — client-side JS must be able to read it and
// echo it back in the X-CSRF-Token header.
func setCSRFCookie(w http.ResponseWriter, r *http.Request, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

// clearCookie clears a cookie by name (used for both session and CSRF
// cookies on logout).
func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// RequireAuth 401s if no authenticated user is present on the request
// context (i.e. withSession didn't resolve one).
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin 403s if the authenticated user isn't an admin. Compose as
// RequireAuth(RequireAdmin(handler)) so an unauthenticated request 401s
// rather than 403ing.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFromContext(r.Context())
		if !u.IsAdmin {
			http.Error(w, "admin only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
