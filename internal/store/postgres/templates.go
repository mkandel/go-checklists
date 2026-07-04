package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mkandel/go-checklists/internal/domain"
)

// TemplateRepo is the Postgres-backed implementation of domain.TemplateRepo.
// Templates are immutable and versioned: CreateVersion always inserts a new
// row rather than mutating an existing one.
type TemplateRepo struct {
	db dbtx
}

var _ domain.TemplateRepo = (*TemplateRepo)(nil)

// CreateVersion inserts t as the next version for its (tenant, name) pair
// (t.Version is computed, overwriting whatever the caller passed in), along
// with items. Call this inside Store.WithTx if concurrent version creation
// for the same tenant/name is possible, since the version-number lookup and
// insert must be serialized against each other.
func (r *TemplateRepo) CreateVersion(ctx context.Context, t *domain.Template, items []domain.TemplateItem) error {
	var latest int
	err := r.db.QueryRow(ctx,
		`SELECT version FROM templates WHERE tenant_id = $1 AND name = $2 ORDER BY version DESC LIMIT 1 FOR UPDATE`,
		t.TenantID, t.Name,
	).Scan(&latest)
	switch {
	case err == nil:
		t.Version = latest + 1
	case errors.Is(err, pgx.ErrNoRows):
		t.Version = 1
	default:
		return fmt.Errorf("postgres: determine next template version: %w", err)
	}

	row := r.db.QueryRow(ctx,
		`INSERT INTO templates (tenant_id, name, version) VALUES ($1, $2, $3) RETURNING id`,
		t.TenantID, t.Name, t.Version,
	)
	if err := row.Scan(&t.ID); err != nil {
		return fmt.Errorf("postgres: create template: %w", err)
	}

	for i := range items {
		items[i].TemplateID = t.ID
		items[i].Position = i
		row := r.db.QueryRow(ctx,
			`INSERT INTO template_items (template_id, name, position, validation_ref)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			items[i].TemplateID, items[i].Name, items[i].Position, items[i].ValidationRef,
		)
		if err := row.Scan(&items[i].ID); err != nil {
			return fmt.Errorf("postgres: create template item: %w", err)
		}
	}
	return nil
}

func (r *TemplateRepo) GetLatestByName(ctx context.Context, tenantID int64, name string) (*domain.Template, []domain.TemplateItem, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, tenant_id, name, version FROM templates WHERE tenant_id = $1 AND name = $2 ORDER BY version DESC LIMIT 1`,
		tenantID, name)
	var t domain.Template
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version); err != nil {
		return nil, nil, fmt.Errorf("postgres: get latest template: %w", err)
	}
	items, err := r.itemsForTemplate(ctx, t.ID)
	if err != nil {
		return nil, nil, err
	}
	return &t, items, nil
}

func (r *TemplateRepo) Get(ctx context.Context, tenantID, id int64) (*domain.Template, []domain.TemplateItem, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, tenant_id, name, version FROM templates WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	var t domain.Template
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version); err != nil {
		return nil, nil, fmt.Errorf("postgres: get template: %w", err)
	}
	items, err := r.itemsForTemplate(ctx, t.ID)
	if err != nil {
		return nil, nil, err
	}
	return &t, items, nil
}

func (r *TemplateRepo) List(ctx context.Context, tenantID int64) ([]domain.Template, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, tenant_id, name, version FROM templates WHERE tenant_id = $1 ORDER BY name, version`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list templates: %w", err)
	}
	defer rows.Close()

	var templates []domain.Template
	for rows.Next() {
		var t domain.Template
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version); err != nil {
			return nil, fmt.Errorf("postgres: scan template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list templates: %w", err)
	}
	return templates, nil
}

func (r *TemplateRepo) itemsForTemplate(ctx context.Context, templateID int64) ([]domain.TemplateItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, template_id, name, position, validation_ref
		 FROM template_items WHERE template_id = $1 ORDER BY position`,
		templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list template items: %w", err)
	}
	defer rows.Close()

	var items []domain.TemplateItem
	for rows.Next() {
		var it domain.TemplateItem
		if err := rows.Scan(&it.ID, &it.TemplateID, &it.Name, &it.Position, &it.ValidationRef); err != nil {
			return nil, fmt.Errorf("postgres: scan template item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list template items: %w", err)
	}
	return items, nil
}
