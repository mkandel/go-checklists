//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
)

func mustCreatePasswordResetToken(t *testing.T, userID int64, token string, expiresAt time.Time) *domain.PasswordResetToken {
	t.Helper()
	pt := &domain.PasswordResetToken{Token: token, UserID: userID, ExpiresAt: expiresAt}
	if err := testStore.PasswordResetTokens().Create(context.Background(), pt); err != nil {
		t.Fatalf("create password reset token: %v", err)
	}
	return pt
}

func TestPasswordResetTokenRepo_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Carol", uniqueName(t, "carol"))
	token := uniqueName(t, "token")
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	mustCreatePasswordResetToken(t, user.ID, token, expiresAt)

	got, err := testStore.PasswordResetTokens().Get(ctx, token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserID != user.ID {
		t.Fatalf("UserID = %d, want %d", got.UserID, user.ID)
	}
	if !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v", got.ExpiresAt, expiresAt)
	}
}

func TestPasswordResetTokenRepo_Delete(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Dave", uniqueName(t, "dave"))
	token := uniqueName(t, "token")
	mustCreatePasswordResetToken(t, user.ID, token, time.Now().Add(time.Hour))

	if err := testStore.PasswordResetTokens().Delete(ctx, token); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := testStore.PasswordResetTokens().Get(ctx, token); err == nil {
		t.Fatal("expected token to be gone after delete")
	}
}

func TestPasswordResetTokenRepo_DeleteExpiredRemovesOnlyPastRows(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Erin", uniqueName(t, "erin"))
	expiredToken := uniqueName(t, "expired")
	liveToken := uniqueName(t, "live")
	mustCreatePasswordResetToken(t, user.ID, expiredToken, time.Now().Add(-time.Hour))
	mustCreatePasswordResetToken(t, user.ID, liveToken, time.Now().Add(time.Hour))

	cutoff := time.Now()
	n, err := testStore.PasswordResetTokens().DeleteExpired(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row deleted, got %d", n)
	}

	if _, err := testStore.PasswordResetTokens().Get(ctx, expiredToken); err == nil {
		t.Fatal("expected expired token to be gone")
	}
	if _, err := testStore.PasswordResetTokens().Get(ctx, liveToken); err != nil {
		t.Fatalf("expected live token to survive, got err: %v", err)
	}
}
