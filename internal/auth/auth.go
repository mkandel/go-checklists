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
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mkandel/go-checklists/internal/domain"
)

// SessionTTL is the fixed session lifetime. No sliding renewal — a session
// simply expires 7 days after creation, regardless of activity.
const SessionTTL = 7 * 24 * time.Hour

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
func CurrentUser(ctx context.Context, users domain.UserRepo, sessions domain.SessionRepo, token string) (*domain.User, error) {
	s, err := sessions.Get(ctx, token)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	if time.Now().After(s.ExpiresAt) {
		return nil, ErrSessionNotFound
	}
	u, err := users.GetByID(ctx, s.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth: current user: %w", err)
	}
	return u, nil
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
