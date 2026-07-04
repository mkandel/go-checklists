package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
)

func mustCreateSession(t *testing.T, userID int64, token string, expiresAt time.Time) *domain.Session {
	t.Helper()
	s := &domain.Session{Token: token, UserID: userID, ExpiresAt: expiresAt}
	if err := testStore.Sessions().Create(context.Background(), s); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s
}

func TestSessionRepo_RefreshUpdatesExpiry(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Alice", uniqueName(t, "alice"))
	token := uniqueName(t, "token")
	mustCreateSession(t, user.ID, token, time.Now().Add(time.Hour))

	newExpiry := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Millisecond)
	if err := testStore.Sessions().Refresh(ctx, token, newExpiry); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	got, err := testStore.Sessions().Get(ctx, token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.ExpiresAt.Equal(newExpiry) {
		t.Fatalf("ExpiresAt = %v, want %v", got.ExpiresAt, newExpiry)
	}
}

func TestSessionRepo_DeleteExpiredRemovesOnlyPastRows(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Bob", uniqueName(t, "bob"))
	expiredToken := uniqueName(t, "expired")
	liveToken := uniqueName(t, "live")
	mustCreateSession(t, user.ID, expiredToken, time.Now().Add(-time.Hour))
	mustCreateSession(t, user.ID, liveToken, time.Now().Add(time.Hour))

	cutoff := time.Now()
	n, err := testStore.Sessions().DeleteExpired(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row deleted, got %d", n)
	}

	if _, err := testStore.Sessions().Get(ctx, expiredToken); err == nil {
		t.Fatal("expected expired session to be gone")
	}
	if _, err := testStore.Sessions().Get(ctx, liveToken); err != nil {
		t.Fatalf("expected live session to survive, got err: %v", err)
	}
}
