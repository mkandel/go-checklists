package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/mkandel/go-checklists/internal/domain"
)

// ErrNotificationNotFound is returned by NotificationRepo.MarkRead when id
// doesn't exist, or exists but doesn't belong to the given user — the two
// cases are indistinguishable to the caller so guessing another user's
// notification id can't be used to probe for its existence.
var ErrNotificationNotFound = errors.New("postgres: notification not found")

// NotificationRepo is the Postgres-backed implementation of
// domain.NotificationRepo.
type NotificationRepo struct {
	db dbtx
}

var _ domain.NotificationRepo = (*NotificationRepo)(nil)

func (r *NotificationRepo) Create(ctx context.Context, n *domain.Notification) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO notifications (recipient_user_id, type, checklist_id, actor_user_id, message)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		n.RecipientUserID, n.Type, n.ChecklistID, n.ActorUserID, n.Message,
	)
	if err := row.Scan(&n.ID); err != nil {
		return fmt.Errorf("postgres: create notification: %w", err)
	}
	return nil
}

func (r *NotificationRepo) ListForUser(ctx context.Context, userID int64) ([]domain.Notification, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, recipient_user_id, type, checklist_id, actor_user_id, message, read_at
		 FROM notifications WHERE recipient_user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(&n.ID, &n.RecipientUserID, &n.Type, &n.ChecklistID, &n.ActorUserID, &n.Message, &n.ReadAt); err != nil {
			return nil, fmt.Errorf("postgres: scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list notifications: %w", err)
	}
	return notifications, nil
}

func (r *NotificationRepo) MarkRead(ctx context.Context, id, userID int64) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE id = $1 AND recipient_user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("postgres: mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationNotFound
	}
	return nil
}
