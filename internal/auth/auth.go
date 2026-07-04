// Package auth implements session-based username/password authentication.
// It is framework-agnostic: no net/http here (cookie handling belongs to
// internal/api).
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mkandel/go-checklists/internal/domain"
)

// SessionTTL is the session lifetime, extended on each renewal (see
// renewThreshold below) — a sliding window, not a hard cutoff from creation.
const SessionTTL = 7 * 24 * time.Hour

// renewThreshold is how much of SessionTTL must remain before CurrentUser
// renews a session on access. Renewing at the halfway point (rather than on
// every request) keeps most requests from writing to the sessions table.
const renewThreshold = SessionTTL / 2

var (
	// ErrInvalidCredentials covers both "no such user" and "wrong password" —
	// deliberately not distinguished, to avoid username enumeration.
	ErrInvalidCredentials = errors.New("auth: invalid username or password")
	ErrInactiveUser       = errors.New("auth: user is inactive")
	ErrSessionNotFound    = errors.New("auth: session not found or expired")
)

// HashPassword hashes a plaintext password for storage.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(b), nil
}

// Login verifies username/password against users and, on success, creates
// and persists a new Session in sessions.
func Login(ctx context.Context, users domain.UserRepo, sessions domain.SessionRepo, username, password string) (*domain.Session, error) {
	u, err := users.GetByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !u.IsActive {
		return nil, ErrInactiveUser
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := newSessionToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s := &domain.Session{
		Token:     token,
		UserID:    u.ID,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionTTL),
	}
	if err := sessions.Create(ctx, s); err != nil {
		return nil, fmt.Errorf("auth: login: %w", err)
	}
	return s, nil
}

// Logout deletes the session identified by token. Deleting a nonexistent
// token is not an error (idempotent logout).
func Logout(ctx context.Context, sessions domain.SessionRepo, token string) error {
	return sessions.Delete(ctx, token)
}

// CurrentUser resolves the user behind a session token, checking expiry.
// Returns ErrSessionNotFound for a missing or expired session.
//
// If less than renewThreshold remains on the session, CurrentUser renews it
// to now + SessionTTL and reports renewed = true, so callers (e.g. the HTTP
// cookie) can extend their own copy of the expiry to match.
func CurrentUser(ctx context.Context, users domain.UserRepo, sessions domain.SessionRepo, token string) (u *domain.User, s *domain.Session, renewed bool, err error) {
	s, err = sessions.Get(ctx, token)
	if err != nil {
		return nil, nil, false, ErrSessionNotFound
	}
	now := time.Now()
	if now.After(s.ExpiresAt) {
		return nil, nil, false, ErrSessionNotFound
	}

	if s.ExpiresAt.Sub(now) < renewThreshold {
		newExpiry := now.Add(SessionTTL)
		if err := sessions.Refresh(ctx, s.Token, newExpiry); err != nil {
			return nil, nil, false, fmt.Errorf("auth: current user: renew session: %w", err)
		}
		s.ExpiresAt = newExpiry
		renewed = true
	}

	u, err = users.GetByID(ctx, s.UserID)
	if err != nil {
		return nil, nil, false, fmt.Errorf("auth: current user: %w", err)
	}
	return u, s, renewed, nil
}

// newSessionToken generates a cryptographically random, URL-safe session
// token: 32 random bytes, base64 URL-encoded, used directly as the
// session's primary key.
func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// loginAttemptWindow is an in-memory, single-process fixed window of failed
// login attempts for one key (typically a client IP).
type loginAttemptWindow struct {
	count       int
	windowStart time.Time
}

// LoginLimiter throttles repeated failed logins. It is in-memory only: state
// doesn't survive a restart and isn't shared across multiple server
// instances — acceptable for a single-instance deployment, and simpler than
// persisting attempt counts for a threat this limiter only needs to slow
// down, not eliminate.
type LoginLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*loginAttemptWindow
	maxAttempts int
	window      time.Duration
}

// NewLoginLimiter returns a LoginLimiter allowing maxAttempts failures per
// key within window before Allow starts returning false for that key.
func NewLoginLimiter(maxAttempts int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{
		attempts:    make(map[string]*loginAttemptWindow),
		maxAttempts: maxAttempts,
		window:      window,
	}
}

// Allow reports whether key may attempt a login right now.
func (l *LoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.attempts[key]
	if !ok {
		return true
	}
	if time.Since(w.windowStart) > l.window {
		delete(l.attempts, key)
		return true
	}
	return w.count < l.maxAttempts
}

// RecordFailure records a failed login attempt for key.
func (l *LoginLimiter) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.attempts[key]
	if !ok || time.Since(w.windowStart) > l.window {
		w = &loginAttemptWindow{windowStart: time.Now()}
		l.attempts[key] = w
	}
	w.count++
}

// RecordSuccess clears key's failure count — a successful login resets the
// window.
func (l *LoginLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
