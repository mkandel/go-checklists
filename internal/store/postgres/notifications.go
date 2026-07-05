package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
)

// ErrNotificationNotFound is returned by NotificationRepo.MarkRead when id
// doesn't exist, or exists but doesn't belong to the given tenant/user — the
// cases are indistinguishable to the caller so guessing another user's (or
// tenant's) notification id can't be used to probe for its existence.
var ErrNotificationNotFound = errors.New("postgres: notification not found")

// NotificationRepo is the Postgres-backed implementation of
// domain.NotificationRepo.
type NotificationRepo struct {
	db dbtx
	// onCreate, if set, is called after a successful Create with the
	// recipient's (tenantID, userID) — wired by Store.Notifications() to
	// wake any SSE subscriber. nil is fine (no push, e.g. in tests/scripts).
	onCreate func(tenantID, userID int64)
}

var _ domain.NotificationRepo = (*NotificationRepo)(nil)

func (r *NotificationRepo) Create(ctx context.Context, n *domain.Notification) error {
	if n.EmailStatus == "" {
		n.EmailStatus = domain.EmailStatusPending
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO notifications (tenant_id, recipient_user_id, type, checklist_id, actor_user_id, message)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		n.TenantID, n.RecipientUserID, n.Type, n.ChecklistID, n.ActorUserID, n.Message,
	)
	if err := row.Scan(&n.ID); err != nil {
		return fmt.Errorf("postgres: create notification: %w", err)
	}
	if r.onCreate != nil {
		r.onCreate(n.TenantID, n.RecipientUserID)
	}
	return nil
}

const notificationColumns = `id, tenant_id, recipient_user_id, type, checklist_id, actor_user_id, message, read_at,
	email_status, email_attempts, email_last_error, email_sent_at`

func scanNotification(row rowScanner) (*domain.Notification, error) {
	var n domain.Notification
	if err := row.Scan(&n.ID, &n.TenantID, &n.RecipientUserID, &n.Type, &n.ChecklistID, &n.ActorUserID, &n.Message, &n.ReadAt,
		&n.EmailStatus, &n.EmailAttempts, &n.EmailLastError, &n.EmailSentAt); err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NotificationRepo) ListForUser(ctx context.Context, tenantID, userID int64) ([]domain.Notification, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+notificationColumns+`
		 FROM notifications WHERE tenant_id = $1 AND recipient_user_id = $2 ORDER BY created_at DESC`,
		tenantID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan notification: %w", err)
		}
		notifications = append(notifications, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list notifications: %w", err)
	}
	return notifications, nil
}

func (r *NotificationRepo) MarkRead(ctx context.Context, tenantID, id, userID int64) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE id = $1 AND tenant_id = $2 AND recipient_user_id = $3`,
		id, tenantID, userID)
	if err != nil {
		return fmt.Errorf("postgres: mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// ListPendingEmail is deliberately NOT tenant-scoped — see the doc comment
// on domain.NotificationRepo.ListPendingEmail.
func (r *NotificationRepo) ListPendingEmail(ctx context.Context, limit int) ([]domain.Notification, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+notificationColumns+`
		 FROM notifications WHERE email_status = $1 ORDER BY created_at ASC LIMIT $2`,
		domain.EmailStatusPending, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list pending email notifications: %w", err)
	}
	defer rows.Close()

	var notifications []domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan notification: %w", err)
		}
		notifications = append(notifications, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list pending email notifications: %w", err)
	}
	return notifications, nil
}

func (r *NotificationRepo) MarkEmailSent(ctx context.Context, id int64, sentAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE notifications SET email_status = $1, email_sent_at = $2 WHERE id = $3`,
		domain.EmailStatusSent, sentAt, id)
	if err != nil {
		return fmt.Errorf("postgres: mark email sent: %w", err)
	}
	return nil
}

func (r *NotificationRepo) MarkEmailFailed(ctx context.Context, id int64, errMsg string, maxAttempts int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE notifications
		 SET email_attempts = email_attempts + 1,
		     email_last_error = $1,
		     email_status = CASE WHEN email_attempts + 1 >= $2 THEN $3 ELSE email_status END
		 WHERE id = $4`,
		errMsg, maxAttempts, domain.EmailStatusFailed, id)
	if err != nil {
		return fmt.Errorf("postgres: mark email failed: %w", err)
	}
	return nil
}

func (r *NotificationRepo) MarkEmailSkipped(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE notifications SET email_status = $1 WHERE id = $2`,
		domain.EmailStatusSkipped, id)
	if err != nil {
		return fmt.Errorf("postgres: mark email skipped: %w", err)
	}
	return nil
}
