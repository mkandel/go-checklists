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

func TestUserRepo_SetActive(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Ivan", uniqueName(t, "ivan"))

	if err := testStore.Users().SetActive(ctx, testTenantID, user.ID, false); err != nil {
		t.Fatalf("set active false: %v", err)
	}
	got, err := testStore.Users().GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.IsActive {
		t.Fatalf("IsActive = true, want false after suspend")
	}

	if err := testStore.Users().SetActive(ctx, testTenantID, user.ID, true); err != nil {
		t.Fatalf("set active true: %v", err)
	}
	got, err = testStore.Users().GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if !got.IsActive {
		t.Fatalf("IsActive = false, want true after reactivate")
	}
}

func TestUserRepo_SetActive_WrongTenantIsNoop(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Judy", uniqueName(t, "judy"))

	if err := testStore.Users().SetActive(ctx, testTenantID+999, user.ID, false); err != nil {
		t.Fatalf("set active: %v", err)
	}
	got, err := testStore.Users().GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if !got.IsActive {
		t.Fatalf("IsActive = false, want true — SetActive under the wrong tenant should not have applied")
	}
}
