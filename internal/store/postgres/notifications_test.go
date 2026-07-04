package postgres_test

import (
	"context"
	"errors"
	"testing"

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
