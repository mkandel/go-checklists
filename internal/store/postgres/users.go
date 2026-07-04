package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mkandel/go-checklists/internal/domain"
)

// pgUniqueViolation is the Postgres error code for a unique constraint
// violation (23505).
const pgUniqueViolation = "23505"

// UserRepo is the Postgres-backed implementation of domain.UserRepo.
type UserRepo struct {
	db dbtx
}

var _ domain.UserRepo = (*UserRepo)(nil)

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO users (tenant_id, name, username, password_hash, is_admin, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		u.TenantID, u.Name, u.Username, u.PasswordHash, u.IsAdmin, u.IsActive,
	)
	if err := row.Scan(&u.ID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.ErrUsernameTaken
		}
		return fmt.Errorf("postgres: create user: %w", err)
	}
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, tenant_id, name, username, password_hash, is_admin, is_active FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *UserRepo) GetByUsername(ctx context.Context, tenantID int64, username string) (*domain.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, tenant_id, name, username, password_hash, is_admin, is_active FROM users WHERE tenant_id = $1 AND username = $2`,
		tenantID, username)
	return scanUser(row)
}

func (r *UserRepo) List(ctx context.Context, tenantID int64) ([]domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, tenant_id, name, username, password_hash, is_admin, is_active FROM users WHERE tenant_id = $1 ORDER BY id`,
		tenantID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Name, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.IsActive); err != nil {
			return nil, fmt.Errorf("postgres: scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	return users, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*domain.User, error) {
	var u domain.User
	if err := row.Scan(&u.ID, &u.TenantID, &u.Name, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.IsActive); err != nil {
		return nil, fmt.Errorf("postgres: get user: %w", err)
	}
	return &u, nil
}
