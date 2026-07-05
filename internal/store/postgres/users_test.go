//go:build integration

package postgres_test

import (
	"context"
	"testing"
)

func TestUserRepo_UpdatePasswordHash(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Heidi", uniqueName(t, "heidi"))

	if err := testStore.Users().UpdatePasswordHash(ctx, user.ID, "new-hash"); err != nil {
		t.Fatalf("update password hash: %v", err)
	}

	got, err := testStore.Users().GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.PasswordHash != "new-hash" {
		t.Fatalf("PasswordHash = %q, want %q", got.PasswordHash, "new-hash")
	}
}
