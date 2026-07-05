//go:build integration

package web_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestGroupsPageRequiresAuth(t *testing.T) {
	srv := newTestServer(t)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(srv.URL + "/groups")
	if err != nil {
		t.Fatalf("get /groups: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
}

func TestCreateGroupAsAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	groupName := uniqueName(t, "NewGroup")
	resp := doForm(t, client, http.MethodPost, srv.URL+"/groups", url.Values{"name": {groupName}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), groupName) {
		t.Errorf("groups table missing %q:\n%s", groupName, body)
	}
}

func TestCreateGroupAsNonAdminForbidden(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	resp := doForm(t, client, http.MethodPost, srv.URL+"/groups", url.Values{"name": {"whatever"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestGroupMembersAddAndRemove(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	member := mustCreateUser(t, uniqueName(t, "member"), "hunter22", true)
	client := mustLogin(t, srv, admin.Username, "hunter22")
	group := mustCreateGroup(t, uniqueName(t, "Ops"))

	membersURL := fmt.Sprintf("%s/groups/%d/members", srv.URL, group.ID)
	addResp := doForm(t, client, http.MethodPost, membersURL, url.Values{"user_id": {strconv.FormatInt(member.ID, 10)}})
	defer addResp.Body.Close()
	if addResp.StatusCode != http.StatusOK {
		t.Fatalf("add member status = %d, want 200", addResp.StatusCode)
	}
	body, _ := io.ReadAll(addResp.Body)
	if !strings.Contains(string(body), member.Name) {
		t.Errorf("members fragment missing %q after add:\n%s", member.Name, body)
	}

	removeURL := fmt.Sprintf("%s/groups/%d/members/%d", srv.URL, group.ID, member.ID)
	req, err := http.NewRequest(http.MethodDelete, removeURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token := csrfTokenFromJar(t, client, removeURL); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	removeResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("remove member: %v", err)
	}
	defer removeResp.Body.Close()
	if removeResp.StatusCode != http.StatusOK {
		t.Fatalf("remove member status = %d, want 200", removeResp.StatusCode)
	}
	afterBody, _ := io.ReadAll(removeResp.Body)
	// Only inspect the members table itself, not the "add member" dropdown —
	// the dropdown legitimately still lists the removed user as a candidate
	// to add back.
	table, _, _ := strings.Cut(string(afterBody), "</table>")
	if strings.Contains(table, member.Name) {
		t.Errorf("members table still contains %q after remove:\n%s", member.Name, afterBody)
	}
	if !strings.Contains(table, "No members yet.") {
		t.Errorf("members table missing empty-state message after remove:\n%s", afterBody)
	}
}

func TestGroupMembersFragmentRequiresAuth(t *testing.T) {
	srv := newTestServer(t)
	group := mustCreateGroup(t, uniqueName(t, "Ops"))
	resp, err := http.Get(fmt.Sprintf("%s/groups/%d/members", srv.URL, group.ID))
	if err != nil {
		t.Fatalf("get members: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
