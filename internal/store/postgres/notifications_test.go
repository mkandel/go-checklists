//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

func TestNotificationRepo_MarkReadSucceedsForOwner(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))

	n := &domain.Notification{TenantID: testTenantID, RecipientUserID: recipient.ID, Type: "test", Message: "hi"}
	if err := testStore.Notifications().Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	if err := testStore.Notifications().MarkRead(ctx, testTenantID, n.ID, recipient.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	got, err := testStore.Notifications().ListForUser(ctx, testTenantID, recipient.ID)
	if err != nil {
		t.Fatalf("list for user: %v", err)
	}
	found := false
	for _, item := range got {
		if item.ID == n.ID {
			if item.ReadAt == nil {
				t.Fatalf("expected notification %d to have ReadAt set", n.ID)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected notification %d in recipient's list", n.ID)
	}
}

func TestNotificationRepo_MarkReadRejectsNonOwner(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))
	other := mustCreateUser(t, "Other", uniqueName(t, "other"))

	n := &domain.Notification{TenantID: testTenantID, RecipientUserID: recipient.ID, Type: "test", Message: "hi"}
	if err := testStore.Notifications().Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	err := testStore.Notifications().MarkRead(ctx, testTenantID, n.ID, other.ID)
	if !errors.Is(err, postgres.ErrNotificationNotFound) {
		t.Fatalf("expected ErrNotificationNotFound for non-owner, got %v", err)
	}

	got, err := testStore.Notifications().ListForUser(ctx, testTenantID, recipient.ID)
	if err != nil {
		t.Fatalf("list for user: %v", err)
	}
	for _, item := range got {
		if item.ID == n.ID && item.ReadAt != nil {
			t.Fatalf("expected notification to remain unread after a non-owner's attempt")
		}
	}
}

func TestNotificationRepo_MarkReadUnknownID(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))

	err := testStore.Notifications().MarkRead(ctx, testTenantID, 0, recipient.ID)
	if !errors.Is(err, postgres.ErrNotificationNotFound) {
		t.Fatalf("expected ErrNotificationNotFound for unknown id, got %v", err)
	}
}

func TestNotificationRepo_NewNotificationStartsEmailPending(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))

	n := &domain.Notification{TenantID: testTenantID, RecipientUserID: recipient.ID, Type: "test", Message: "hi"}
	if err := testStore.Notifications().Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if n.EmailStatus != domain.EmailStatusPending {
		t.Fatalf("expected new notification EmailStatus %q, got %q", domain.EmailStatusPending, n.EmailStatus)
	}
	if n.EmailAttempts != 0 {
		t.Fatalf("expected new notification EmailAttempts 0, got %d", n.EmailAttempts)
	}
}

func TestNotificationRepo_ListPendingEmailIncludesNewNotification(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))

	n := &domain.Notification{TenantID: testTenantID, RecipientUserID: recipient.ID, Type: "test", Message: "pending one"}
	if err := testStore.Notifications().Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	pending, err := testStore.Notifications().ListPendingEmail(ctx, 1000)
	if err != nil {
		t.Fatalf("list pending email: %v", err)
	}
	found := false
	for _, item := range pending {
		if item.ID == n.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected notification %d in pending email list", n.ID)
	}
}

func TestNotificationRepo_MarkEmailSent(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))

	n := &domain.Notification{TenantID: testTenantID, RecipientUserID: recipient.ID, Type: "test", Message: "hi"}
	if err := testStore.Notifications().Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	sentAt := time.Now().Truncate(time.Second)
	if err := testStore.Notifications().MarkEmailSent(ctx, n.ID, sentAt); err != nil {
		t.Fatalf("mark email sent: %v", err)
	}

	got := mustGetNotification(t, recipient.ID, n.ID)
	if got.EmailStatus != domain.EmailStatusSent {
		t.Fatalf("expected EmailStatus %q, got %q", domain.EmailStatusSent, got.EmailStatus)
	}
	if got.EmailSentAt == nil {
		t.Fatalf("expected EmailSentAt to be set")
	}
}

func TestNotificationRepo_MarkEmailFailedRetriesThenGivesUp(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))

	n := &domain.Notification{TenantID: testTenantID, RecipientUserID: recipient.ID, Type: "test", Message: "hi"}
	if err := testStore.Notifications().Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	const maxAttempts = 3
	for i := 1; i < maxAttempts; i++ {
		if err := testStore.Notifications().MarkEmailFailed(ctx, n.ID, "boom", maxAttempts); err != nil {
			t.Fatalf("mark email failed (attempt %d): %v", i, err)
		}
		got := mustGetNotification(t, recipient.ID, n.ID)
		if got.EmailStatus != domain.EmailStatusPending {
			t.Fatalf("attempt %d: expected EmailStatus to stay %q, got %q", i, domain.EmailStatusPending, got.EmailStatus)
		}
		if got.EmailAttempts != i {
			t.Fatalf("attempt %d: expected EmailAttempts %d, got %d", i, i, got.EmailAttempts)
		}
	}

	if err := testStore.Notifications().MarkEmailFailed(ctx, n.ID, "boom", maxAttempts); err != nil {
		t.Fatalf("final mark email failed: %v", err)
	}
	got := mustGetNotification(t, recipient.ID, n.ID)
	if got.EmailStatus != domain.EmailStatusFailed {
		t.Fatalf("expected EmailStatus %q after reaching maxAttempts, got %q", domain.EmailStatusFailed, got.EmailStatus)
	}
	if got.EmailAttempts != maxAttempts {
		t.Fatalf("expected EmailAttempts %d, got %d", maxAttempts, got.EmailAttempts)
	}
	if got.EmailLastError == nil || *got.EmailLastError != "boom" {
		t.Fatalf("expected EmailLastError %q, got %v", "boom", got.EmailLastError)
	}
}

func TestNotificationRepo_MarkEmailSkipped(t *testing.T) {
	ctx := context.Background()
	recipient := mustCreateUser(t, "Recipient", uniqueName(t, "recipient"))

	n := &domain.Notification{TenantID: testTenantID, RecipientUserID: recipient.ID, Type: "test", Message: "hi"}
	if err := testStore.Notifications().Create(ctx, n); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	if err := testStore.Notifications().MarkEmailSkipped(ctx, n.ID); err != nil {
		t.Fatalf("mark email skipped: %v", err)
	}
	got := mustGetNotification(t, recipient.ID, n.ID)
	if got.EmailStatus != domain.EmailStatusSkipped {
		t.Fatalf("expected EmailStatus %q, got %q", domain.EmailStatusSkipped, got.EmailStatus)
	}
}

// mustGetNotification fetches notification id via ListForUser (recipientID
// is the notification's RecipientUserID) rather than ListPendingEmail,
// since ListPendingEmail only returns rows still in EmailStatusPending —
// no good for inspecting state after a Mark* call has moved it elsewhere.
func mustGetNotification(t *testing.T, recipientID, id int64) domain.Notification {
	t.Helper()
	all, err := testStore.Notifications().ListForUser(context.Background(), testTenantID, recipientID)
	if err != nil {
		t.Fatalf("list for user: %v", err)
	}
	for _, n := range all {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("notification %d not found for recipient %d", id, recipientID)
	return domain.Notification{}
}
