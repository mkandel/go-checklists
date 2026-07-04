package postgres

import (
	"context"
	"fmt"

	"github.com/mkandel/go-checklists/internal/domain"
)

// GroupRepo is the Postgres-backed implementation of domain.GroupRepo.
type GroupRepo struct {
	db dbtx
}

var _ domain.GroupRepo = (*GroupRepo)(nil)

func (r *GroupRepo) Create(ctx context.Context, g *domain.Group) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO groups (tenant_id, name) VALUES ($1, $2) RETURNING id`, g.TenantID, g.Name)
	if err := row.Scan(&g.ID); err != nil {
		return fmt.Errorf("postgres: create group: %w", err)
	}
	return nil
}

func (r *GroupRepo) AddMember(ctx context.Context, tenantID, groupID, userID int64) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO user_groups (user_id, group_id)
		 SELECT u.id, g.id FROM users u, groups g
		 WHERE u.id = $1 AND u.tenant_id = $2 AND g.id = $3 AND g.tenant_id = $2
		 ON CONFLICT DO NOTHING`,
		userID, tenantID, groupID,
	)
	if err != nil {
		return fmt.Errorf("postgres: add group member: %w", err)
	}
	return nil
}

func (r *GroupRepo) RemoveMember(ctx context.Context, tenantID, groupID, userID int64) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM user_groups ug
		 USING users u, groups g
		 WHERE ug.user_id = u.id AND ug.group_id = g.id
		   AND ug.user_id = $1 AND ug.group_id = $2
		   AND u.tenant_id = $3 AND g.tenant_id = $3`,
		userID, groupID, tenantID,
	)
	if err != nil {
		return fmt.Errorf("postgres: remove group member: %w", err)
	}
	return nil
}

func (r *GroupRepo) IsMember(ctx context.Context, tenantID, groupID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM user_groups ug
			JOIN users u ON u.id = ug.user_id
			JOIN groups g ON g.id = ug.group_id
			WHERE ug.user_id = $1 AND ug.group_id = $2 AND u.tenant_id = $3 AND g.tenant_id = $3
		)`,
		userID, groupID, tenantID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: check group membership: %w", err)
	}
	return exists, nil
}

func (r *GroupRepo) List(ctx context.Context, tenantID int64) ([]domain.Group, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, tenant_id, name FROM groups WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list groups: %w", err)
	}
	defer rows.Close()

	var groups []domain.Group
	for rows.Next() {
		var g domain.Group
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name); err != nil {
			return nil, fmt.Errorf("postgres: scan group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list groups: %w", err)
	}
	return groups, nil
}

func (r *GroupRepo) ListMembers(ctx context.Context, tenantID, groupID int64) ([]domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT u.id, u.tenant_id, u.name, u.username, u.password_hash, u.is_admin, u.is_active
		 FROM users u
		 JOIN user_groups ug ON ug.user_id = u.id
		 JOIN groups g ON g.id = ug.group_id
		 WHERE ug.group_id = $1 AND g.tenant_id = $2
		 ORDER BY u.id`,
		groupID, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list group members: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Name, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.IsActive); err != nil {
			return nil, fmt.Errorf("postgres: scan group member: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list group members: %w", err)
	}
	return users, nil
}
