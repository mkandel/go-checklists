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
		`INSERT INTO users (tenant_id, name, username, password_hash, email, is_admin, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		u.TenantID, u.Name, u.Username, u.PasswordHash, u.Email, u.IsAdmin, u.IsActive,
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
		`SELECT id, tenant_id, name, username, password_hash, email, is_admin, is_active FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *UserRepo) GetByUsername(ctx context.Context, tenantID int64, username string) (*domain.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, tenant_id, name, username, password_hash, email, is_admin, is_active FROM users WHERE tenant_id = $1 AND username = $2`,
		tenantID, username)
	return scanUser(row)
}

func (r *UserRepo) List(ctx context.Context, tenantID int64) ([]domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, tenant_id, name, username, password_hash, email, is_admin, is_active FROM users WHERE tenant_id = $1 ORDER BY id`,
		tenantID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Name, &u.Username, &u.PasswordHash, &u.Email, &u.IsAdmin, &u.IsActive); err != nil {
			return nil, fmt.Errorf("postgres: scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	return users, nil
}

// userSortColumns allowlists the columns UserFilter.SortBy may name, so user
// input never gets interpolated directly into the ORDER BY clause.
var userSortColumns = map[string]string{
	"name":      "name",
	"username":  "username",
	"email":     "email",
	"is_admin":  "is_admin",
	"is_active": "is_active",
}

func (r *UserRepo) ListFiltered(ctx context.Context, filter domain.UserFilter) ([]domain.User, error) {
	orderCol, ok := userSortColumns[filter.SortBy]
	if !ok {
		orderCol = "id"
	}
	orderDir := "ASC"
	if filter.SortDir == "desc" {
		orderDir = "DESC"
	}

	rows, err := r.db.Query(ctx,
		`SELECT id, tenant_id, name, username, password_hash, email, is_admin, is_active FROM users
		 WHERE tenant_id = $1 AND ($2 OR is_active)
		 ORDER BY `+orderCol+` `+orderDir+`, id`,
		filter.TenantID, filter.IncludeInactive)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Name, &u.Username, &u.PasswordHash, &u.Email, &u.IsAdmin, &u.IsActive); err != nil {
			return nil, fmt.Errorf("postgres: scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	return users, nil
}

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, userID int64, hash string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, userID)
	if err != nil {
		return fmt.Errorf("postgres: update password hash: %w", err)
	}
	return nil
}

func (r *UserRepo) SetActive(ctx context.Context, tenantID, userID int64, active bool) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET is_active = $1 WHERE id = $2 AND tenant_id = $3`,
		active, userID, tenantID)
	if err != nil {
		return fmt.Errorf("postgres: set user active: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*domain.User, error) {
	var u domain.User
	if err := row.Scan(&u.ID, &u.TenantID, &u.Name, &u.Username, &u.PasswordHash, &u.Email, &u.IsAdmin, &u.IsActive); err != nil {
		return nil, fmt.Errorf("postgres: get user: %w", err)
	}
	return &u, nil
}
