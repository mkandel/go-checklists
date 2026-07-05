package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
)

// PasswordResetTokenRepo is the Postgres-backed implementation of
// domain.PasswordResetTokenRepo.
type PasswordResetTokenRepo struct {
	db dbtx
}

var _ domain.PasswordResetTokenRepo = (*PasswordResetTokenRepo)(nil)

func (r *PasswordResetTokenRepo) Create(ctx context.Context, t *domain.PasswordResetToken) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO password_reset_tokens (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		t.Token, t.UserID, t.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: create password reset token: %w", err)
	}
	return nil
}

func (r *PasswordResetTokenRepo) Get(ctx context.Context, token string) (*domain.PasswordResetToken, error) {
	var t domain.PasswordResetToken
	err := r.db.QueryRow(ctx,
		`SELECT token, user_id, created_at, expires_at FROM password_reset_tokens WHERE token = $1`, token,
	).Scan(&t.Token, &t.UserID, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("postgres: get password reset token: %w", err)
	}
	return &t, nil
}

func (r *PasswordResetTokenRepo) Delete(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM password_reset_tokens WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("postgres: delete password reset token: %w", err)
	}
	return nil
}

func (r *PasswordResetTokenRepo) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < $1`, now)
	if err != nil {
		return 0, fmt.Errorf("postgres: delete expired password reset tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}
