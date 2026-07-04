//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

// mustOpenRawDB opens a direct connection to the test database, bypassing
// the repo layer, for tests that need to inspect state the repos
// deliberately hide (e.g. soft-deleted checklist_items rows).
func mustOpenRawDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// uniqueName derives a value unique to the calling test (via t.Name()) so
// tests sharing one database don't collide on unique constraints like
// users.username or templates(name, version).
func uniqueName(t *testing.T, suffix string) string {
	t.Helper()
	return strings.ReplaceAll(t.Name(), "/", "_") + "_" + suffix
}

func mustCreateUser(t *testing.T, name, username string) *domain.User {
	t.Helper()
	u := &domain.User{TenantID: testTenantID, Name: name, Username: username, IsActive: true}
	if err := testStore.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u
}

func mustCreateGroup(t *testing.T, name string, memberIDs ...int64) *domain.Group {
	t.Helper()
	ctx := context.Background()
	g := &domain.Group{TenantID: testTenantID, Name: name}
	if err := testStore.Groups().Create(ctx, g); err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	for _, uid := range memberIDs {
		if err := testStore.Groups().AddMember(ctx, testTenantID, g.ID, uid); err != nil {
			t.Fatalf("add member %d to group %s: %v", uid, name, err)
		}
	}
	return g
}
