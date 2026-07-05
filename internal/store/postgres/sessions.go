package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
)

// SessionRepo is the Postgres-backed implementation of domain.SessionRepo.
type SessionRepo struct {
	db dbtx
}

var _ domain.SessionRepo = (*SessionRepo)(nil)

func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		s.Token, s.UserID, s.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	return nil
}

func (r *SessionRepo) Get(ctx context.Context, token string) (*domain.Session, error) {
	var s domain.Session
	err := r.db.QueryRow(ctx,
		`SELECT token, user_id, created_at, expires_at FROM sessions WHERE token = $1`, token,
	).Scan(&s.Token, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("postgres: get session: %w", err)
	}
	return &s, nil
}

func (r *SessionRepo) Delete(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("postgres: delete session: %w", err)
	}
	return nil
}

func (r *SessionRepo) Refresh(ctx context.Context, token string, newExpiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE sessions SET expires_at = $1 WHERE token = $2`,
		newExpiresAt, token,
	)
	if err != nil {
		return fmt.Errorf("postgres: refresh session: %w", err)
	}
	return nil
}

func (r *SessionRepo) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < $1`, now)
	if err != nil {
		return 0, fmt.Errorf("postgres: delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *SessionRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("postgres: delete sessions by user: %w", err)
	}
	return nil
}
