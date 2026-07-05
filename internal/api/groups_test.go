//go:build integration

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func TestGroups_ReadRoutesAvailableToAnyAuthedUser(t *testing.T) {
	srv := newTestServer(t)
	memberName := uniqueName(t, "member")
	member := mustCreateUser(t, memberName, "hunter2", true)
	group := mustCreateGroup(t, uniqueName(t, "team"), member.ID)

	callerName := uniqueName(t, "caller")
	mustCreateUser(t, callerName, "hunter2", true)
	client := mustLogin(t, srv, callerName, "hunter2")

	listResp := doJSON(t, client, http.MethodGet, srv.URL+"/api/groups", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list groups status = %d, want 200", listResp.StatusCode)
	}
	defer listResp.Body.Close()
	var groups []domain.Group
	if err := json.NewDecoder(listResp.Body).Decode(&groups); err != nil {
		t.Fatalf("decode groups: %v", err)
	}
	found := false
	for _, g := range groups {
		if g.ID == group.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected group %d in list, got %+v", group.ID, groups)
	}

	membersResp := doJSON(t, client, http.MethodGet, fmt.Sprintf("%s/api/groups/%d/members", srv.URL, group.ID), nil)
	if membersResp.StatusCode != http.StatusOK {
		t.Fatalf("list members status = %d, want 200", membersResp.StatusCode)
	}
	defer membersResp.Body.Close()
	var members []domain.User
	if err := json.NewDecoder(membersResp.Body).Decode(&members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 1 || members[0].ID != member.ID {
		t.Fatalf("expected only %d as member, got %+v", member.ID, members)
	}
}

func TestGroups_MutationsAreAdminOnly(t *testing.T) {
	srv := newTestServer(t)
	nonAdminName := uniqueName(t, "alice")
	mustCreateUser(t, nonAdminName, "hunter2", true)
	nonAdminClient := mustLogin(t, srv, nonAdminName, "hunter2")

	createResp := doJSON(t, nonAdminClient, http.MethodPost, srv.URL+"/api/groups", map[string]any{
		"name": uniqueName(t, "team"),
	})
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin create group status = %d, want 403", createResp.StatusCode)
	}

	adminName := uniqueName(t, "admin")
	mustCreateAdminUser(t, adminName, "hunter2")
	adminClient := mustLogin(t, srv, adminName, "hunter2")

	adminCreateResp := doJSON(t, adminClient, http.MethodPost, srv.URL+"/api/groups", map[string]any{
		"name": uniqueName(t, "team"),
	})
	if adminCreateResp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create group status = %d, want 201", adminCreateResp.StatusCode)
	}
	defer adminCreateResp.Body.Close()
	var group domain.Group
	if err := json.NewDecoder(adminCreateResp.Body).Decode(&group); err != nil {
		t.Fatalf("decode group: %v", err)
	}

	memberName := uniqueName(t, "member")
	member := mustCreateUser(t, memberName, "hunter2", true)

	addForbidden := doJSON(t, nonAdminClient, http.MethodPost,
		fmt.Sprintf("%s/api/groups/%d/members", srv.URL, group.ID),
		map[string]any{"user_id": member.ID})
	defer addForbidden.Body.Close()
	if addForbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin add member status = %d, want 403", addForbidden.StatusCode)
	}

	addResp := doJSON(t, adminClient, http.MethodPost,
		fmt.Sprintf("%s/api/groups/%d/members", srv.URL, group.ID),
		map[string]any{"user_id": member.ID})
	defer addResp.Body.Close()
	if addResp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin add member status = %d, want 204", addResp.StatusCode)
	}

	removeForbidden := doJSON(t, nonAdminClient, http.MethodDelete,
		fmt.Sprintf("%s/api/groups/%d/members/%d", srv.URL, group.ID, member.ID), nil)
	defer removeForbidden.Body.Close()
	if removeForbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin remove member status = %d, want 403", removeForbidden.StatusCode)
	}

	removeResp := doJSON(t, adminClient, http.MethodDelete,
		fmt.Sprintf("%s/api/groups/%d/members/%d", srv.URL, group.ID, member.ID), nil)
	defer removeResp.Body.Close()
	if removeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin remove member status = %d, want 204", removeResp.StatusCode)
	}
}
