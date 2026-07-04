package api

import (
	"context"
	"net/http"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
)

const sessionCookieName = "checklists_session"

type contextKey int

const userContextKey contextKey = 0

// UserFromContext returns the authenticated user attached to ctx by
// withSession, if any.
func UserFromContext(ctx context.Context) (*domain.User, bool) {
	u, ok := ctx.Value(userContextKey).(*domain.User)
	return u, ok
}

// withSession wraps next, resolving the session cookie (if present) into a
// *domain.User and attaching it to the request context. It never rejects a
// request on its own — use RequireAuth to enforce authentication on
// specific routes.
func withSession(users domain.UserRepo, sessions domain.SessionRepo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		u, err := auth.CurrentUser(r.Context(), users, sessions, cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
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
