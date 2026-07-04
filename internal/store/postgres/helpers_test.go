package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

// uniqueName derives a value unique to the calling test (via t.Name()) so
// tests sharing one database don't collide on unique constraints like
// users.username or templates(name, version).
func uniqueName(t *testing.T, suffix string) string {
	t.Helper()
	return strings.ReplaceAll(t.Name(), "/", "_") + "_" + suffix
}

func mustCreateUser(t *testing.T, name, username string) *domain.User {
	t.Helper()
	u := &domain.User{Name: name, Username: username, IsActive: true}
	if err := testStore.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u
}

func mustCreateGroup(t *testing.T, name string, memberIDs ...int64) *domain.Group {
	t.Helper()
	ctx := context.Background()
	g := &domain.Group{Name: name}
	if err := testStore.Groups().Create(ctx, g); err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	for _, uid := range memberIDs {
		if err := testStore.Groups().AddMember(ctx, g.ID, uid); err != nil {
			t.Fatalf("add member %d to group %s: %v", uid, name, err)
		}
	}
	return g
}
