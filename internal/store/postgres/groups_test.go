//go:build integration

package postgres_test

import (
	"context"
	"testing"
)

func TestGroupRepo_ListReturnsCreatedGroups(t *testing.T) {
	ctx := context.Background()
	g1 := mustCreateGroup(t, uniqueName(t, "alpha"))
	g2 := mustCreateGroup(t, uniqueName(t, "beta"))

	got, err := testStore.Groups().List(ctx, testTenantID)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}

	ids := make(map[int64]bool, len(got))
	for _, g := range got {
		ids[g.ID] = true
	}
	if !ids[g1.ID] || !ids[g2.ID] {
		t.Fatalf("expected both created groups in list, got %+v", got)
	}
}
