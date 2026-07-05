package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/mail"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// maxPasswordResetAttempts and passwordResetAttemptWindow bound how many
// password-reset requests a single client IP may make before
// handlePasswordResetRequest starts responding 429 — prevents email-bombing
// an address. Mirrors maxLoginAttempts/loginAttemptWindow's shape, but this
// is a separate limiter instance since the two endpoints protect different
// resources.
const (
	maxPasswordResetAttempts   = 5
	passwordResetAttemptWindow = 15 * time.Minute
)

// registerPasswordResetRoutes wires the forgot-password endpoints onto mux.
// Called from RegisterAuthRoutes alongside /login, /register, and /logout so
// these unprefixed, unauthenticated routes are available on both the API and
// web muxes without internal/web importing internal/api.
func registerPasswordResetRoutes(mux *http.ServeMux, store *postgres.Store) {
	limiter := auth.NewLoginLimiter(maxPasswordResetAttempts, passwordResetAttemptWindow)
	mux.HandleFunc("POST /password-reset/request", handlePasswordResetRequest(store, limiter))
	mux.HandleFunc("POST /password-reset/confirm", handlePasswordResetConfirm(store))
}

// handlePasswordResetRequest always responds 204 regardless of whether
// username matched an account, that account is active, or has an email on
// file — differences are only observable server-side via logs. This mirrors
// auth.ErrInvalidCredentials's enumeration-safe philosophy: a client must not
// be able to tell which accounts exist by probing this endpoint.
func handlePasswordResetRequest(store *postgres.Store, limiter *auth.LoginLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := loginRateLimitKey(r)
		if !limiter.Allow(key) {
			http.Error(w, "too many requests, try again later", http.StatusTooManyRequests)
			return
		}
		limiter.RecordFailure(key)

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		username := r.FormValue("username")

		tenant, err := store.Tenants().GetSoleTenant(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		token, u, err := auth.RequestPasswordReset(r.Context(), store.Users(), store.PasswordResetTokens(), tenant.ID, username)
		if err != nil {
			if !errors.Is(err, auth.ErrPasswordResetNotSendable) {
				log.Printf("password reset: request for %q: %v", username, err)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if err := sendPasswordResetEmail(r, tenant, u, token); err != nil {
			log.Printf("password reset: send email for user %d: %v", u.ID, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// sendPasswordResetEmail builds an absolute reset-confirm link from the
// incoming request's own host/scheme (so the link works whichever port —
// API or web — the request arrived on, matching the "sessions valid on
// either port" symmetry) and sends it synchronously via mail.Send. Unlike
// the async notification outbox (cmd/checklists-server/email_delivery.go),
// a user requesting a reset is actively waiting on this email.
func sendPasswordResetEmail(r *http.Request, tenant *domain.Tenant, u *domain.User, token *domain.PasswordResetToken) error {
	if tenant.SMTPHost == nil {
		return fmt.Errorf("tenant %d has no SMTP config", tenant.ID)
	}
	if u.Email == nil {
		return fmt.Errorf("user %d has no email", u.ID)
	}

	scheme := "http"
	if isSecureRequest(r) {
		scheme = "https"
	}
	link := fmt.Sprintf("%s://%s/password-reset/confirm?token=%s", scheme, r.Host, token.Token)

	cfg := mail.SMTPConfig{
		Host:        *tenant.SMTPHost,
		Username:    strPtrValue(tenant.SMTPUsername),
		Password:    tenant.SMTPPassword,
		FromAddress: strPtrValue(tenant.SMTPFromAddress),
	}
	if tenant.SMTPPort != nil {
		cfg.Port = *tenant.SMTPPort
	}
	msg := mail.Message{
		To:      *u.Email,
		Subject: "Reset your password",
		Body:    fmt.Sprintf("Follow this link to reset your password:\n\n%s\n\nThis link expires in one hour. If you didn't request this, you can ignore this email.", link),
	}
	return mail.Send(cfg, msg)
}

// strPtrValue mirrors cmd/checklists-server/email_delivery.go's identical
// helper: returns "" for a nil pointer.
func strPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type passwordResetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// handlePasswordResetConfirm sets a new password from a valid, unexpired
// reset token, then logs the user into a fresh session — mirroring
// handleLogin/handleRegister's cookie setup. Unlike the request endpoint, an
// invalid/expired token responds with a direct error: an unguessable random
// token isn't enumeration-sensitive the way a username is.
func handlePasswordResetConfirm(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req passwordResetConfirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		if len(req.Password) < minRegisterPasswordLength {
			http.Error(w, fmt.Sprintf("password must be at least %d characters", minRegisterPasswordLength), http.StatusBadRequest)
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		u, err := auth.ConfirmPasswordReset(r.Context(), store.Users(), store.PasswordResetTokens(), store.Sessions(), req.Token, hash)
		if err != nil {
			if errors.Is(err, auth.ErrPasswordResetTokenNotFound) {
				http.Error(w, "invalid or expired token", http.StatusBadRequest)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		session, err := auth.Login(r.Context(), store.Users(), store.Sessions(), u.TenantID, u.Username, req.Password)
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
		w.WriteHeader(http.StatusNoContent)
	}
}
