package postgres

import (
	"context"
	"fmt"

	"github.com/mkandel/go-checklists/internal/domain"
)

type TenantRepo struct {
	db dbtx
}

var _ domain.TenantRepo = (*TenantRepo)(nil)

func (r *TenantRepo) Create(ctx context.Context, t *domain.Tenant) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		t.Name, t.Slug,
	)
	if err := row.Scan(&t.ID); err != nil {
		return fmt.Errorf("postgres: create tenant: %w", err)
	}
	return nil
}

func (r *TenantRepo) GetByID(ctx context.Context, id int64) (*domain.Tenant, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, name, slug FROM tenants WHERE id = $1`, id)
	var t domain.Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.Slug); err != nil {
		return nil, fmt.Errorf("postgres: get tenant: %w", err)
	}
	return &t, nil
}

func (r *TenantRepo) GetSoleTenant(ctx context.Context) (*domain.Tenant, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, slug FROM tenants ORDER BY id LIMIT 2`)
	if err != nil {
		return nil, fmt.Errorf("postgres: get sole tenant: %w", err)
	}
	defer rows.Close()

	var tenants []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug); err != nil {
			return nil, fmt.Errorf("postgres: get sole tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: get sole tenant: %w", err)
	}

	if len(tenants) != 1 {
		return nil, fmt.Errorf("postgres: get sole tenant: expected exactly 1 tenant, found %d", len(tenants))
	}
	return &tenants[0], nil
}
