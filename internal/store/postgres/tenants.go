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

const tenantColumns = `id, name, slug, smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_address, restrict_checklist_creation, checklist_creator_group_id`

func scanTenant(row rowScanner) (*domain.Tenant, error) {
	var t domain.Tenant
	var smtpPassword *string
	if err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.SMTPHost, &t.SMTPPort, &t.SMTPUsername, &smtpPassword, &t.SMTPFromAddress, &t.RestrictChecklistCreation, &t.CreatorGroupID); err != nil {
		return nil, err
	}
	if smtpPassword != nil {
		t.SMTPPassword = *smtpPassword
	}
	return &t, nil
}

func (r *TenantRepo) GetByID(ctx context.Context, id int64) (*domain.Tenant, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM tenants WHERE id = $1`, id)
	t, err := scanTenant(row)
	if err != nil {
		return nil, fmt.Errorf("postgres: get tenant: %w", err)
	}
	return t, nil
}

func (r *TenantRepo) GetSoleTenant(ctx context.Context) (*domain.Tenant, error) {
	rows, err := r.db.Query(ctx, `SELECT `+tenantColumns+` FROM tenants ORDER BY id LIMIT 2`)
	if err != nil {
		return nil, fmt.Errorf("postgres: get sole tenant: %w", err)
	}
	defer rows.Close()

	var tenants []domain.Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: get sole tenant: %w", err)
		}
		tenants = append(tenants, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: get sole tenant: %w", err)
	}

	if len(tenants) != 1 {
		return nil, fmt.Errorf("postgres: get sole tenant: expected exactly 1 tenant, found %d", len(tenants))
	}
	return &tenants[0], nil
}

// UpdateMailConfig replaces tenantID's SMTP config. An empty cfg.Password
// means "keep the existing password" (COALESCE against the current value)
// so a client never has to round-trip the real secret back just to
// resubmit the rest of the config unchanged.
func (r *TenantRepo) UpdateMailConfig(ctx context.Context, tenantID int64, cfg domain.TenantMailConfig) error {
	var password *string
	if cfg.Password != "" {
		password = &cfg.Password
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE tenants
		 SET smtp_host = $1, smtp_port = $2, smtp_username = $3,
		     smtp_password = COALESCE($4, smtp_password), smtp_from_address = $5
		 WHERE id = $6`,
		cfg.Host, cfg.Port, cfg.Username, password, cfg.FromAddress, tenantID,
	)
	if err != nil {
		return fmt.Errorf("postgres: update tenant mail config: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("postgres: update tenant mail config: tenant %d not found", tenantID)
	}
	return nil
}

// UpdateChecklistCreationPolicy replaces tenantID's checklist-creation
// restriction settings.
func (r *TenantRepo) UpdateChecklistCreationPolicy(ctx context.Context, tenantID int64, policy domain.ChecklistCreationPolicy) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE tenants SET restrict_checklist_creation = $1, checklist_creator_group_id = $2 WHERE id = $3`,
		policy.Restrict, policy.CreatorGroupID, tenantID,
	)
	if err != nil {
		return fmt.Errorf("postgres: update checklist creation policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("postgres: update checklist creation policy: tenant %d not found", tenantID)
	}
	return nil
}
