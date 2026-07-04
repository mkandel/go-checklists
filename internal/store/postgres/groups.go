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
		`INSERT INTO groups (name) VALUES ($1) RETURNING id`, g.Name)
	if err := row.Scan(&g.ID); err != nil {
		return fmt.Errorf("postgres: create group: %w", err)
	}
	return nil
}

func (r *GroupRepo) AddMember(ctx context.Context, groupID, userID int64) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO user_groups (user_id, group_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		userID, groupID,
	)
	if err != nil {
		return fmt.Errorf("postgres: add group member: %w", err)
	}
	return nil
}

func (r *GroupRepo) RemoveMember(ctx context.Context, groupID, userID int64) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM user_groups WHERE user_id = $1 AND group_id = $2`, userID, groupID)
	if err != nil {
		return fmt.Errorf("postgres: remove group member: %w", err)
	}
	return nil
}

func (r *GroupRepo) IsMember(ctx context.Context, groupID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_groups WHERE user_id = $1 AND group_id = $2)`,
		userID, groupID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: check group membership: %w", err)
	}
	return exists, nil
}

func (r *GroupRepo) ListMembers(ctx context.Context, groupID int64) ([]domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT u.id, u.name, u.username, u.password_hash, u.is_admin, u.is_active
		 FROM users u
		 JOIN user_groups ug ON ug.user_id = u.id
		 WHERE ug.group_id = $1
		 ORDER BY u.id`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list group members: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.IsActive); err != nil {
			return nil, fmt.Errorf("postgres: scan group member: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list group members: %w", err)
	}
	return users, nil
}
