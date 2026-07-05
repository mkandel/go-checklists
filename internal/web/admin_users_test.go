//go:build integration

package web_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func TestCreateUserAsAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	username := uniqueName(t, "newuser")
	form := url.Values{
		"username": {username},
		"name":     {"New User"},
		"password": {"hunter22"},
	}
	resp := doForm(t, client, http.MethodPost, srv.URL+"/admin/users", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), username) {
		t.Errorf("users table missing %q:\n%s", username, body)
	}
}

func TestCreateUserAsNonAdminForbidden(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	form := url.Values{"username": {"whoever"}, "name": {"Whoever"}, "password": {"hunter22"}}
	resp := doForm(t, client, http.MethodPost, srv.URL+"/admin/users", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestBulkCreateUsersCSV(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	u1 := uniqueName(t, "bulk1")
	u2 := uniqueName(t, "bulk2")
	csv := u1 + ",hunter22,Bulk One\n" + u2 + ",hunter22,Bulk Two,true\n"

	uploadURL := srv.URL + "/admin/users/bulk"
	req, err := http.NewRequest(http.MethodPost, uploadURL, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "text/csv")
	if token := csrfTokenFromJar(t, client, uploadURL); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("bulk upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), u1) || !strings.Contains(string(body), u2) {
		t.Errorf("bulk result missing created usernames:\n%s", body)
	}
	if !strings.Contains(string(body), "created") {
		t.Errorf("bulk result missing 'created' status:\n%s", body)
	}
}

func TestSuspendAndReactivateUserAsAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	adminClient := mustLogin(t, srv, admin.Username, "hunter22")

	target := mustCreateUser(t, uniqueName(t, "target"), "hunter22", true)
	targetClient := mustLogin(t, srv, target.Username, "hunter22")

	suspendURL := srv.URL + "/admin/users/" + strconv.FormatInt(target.ID, 10) + "/active"
	resp := doForm(t, adminClient, http.MethodPost, suspendURL, url.Values{"active": {"false"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("suspend status = %d, want 200", resp.StatusCode)
	}

	// The target's already-active session should stop working immediately —
	// not just block their next login.
	meResp, err := targetClient.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("get /api/me as suspended user: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("suspended user's /api/me status = %d, want 401", meResp.StatusCode)
	}

	reactivateResp := doForm(t, adminClient, http.MethodPost, suspendURL, url.Values{"active": {"true"}})
	defer reactivateResp.Body.Close()
	if reactivateResp.StatusCode != http.StatusOK {
		t.Fatalf("reactivate status = %d, want 200", reactivateResp.StatusCode)
	}

	// A fresh login should now succeed again.
	newClient := mustLogin(t, srv, target.Username, "hunter22")
	if newClient == nil {
		t.Fatal("expected reactivated user to be able to log in again")
	}
}

func TestSuspendUserClearsChecklistAssignments(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	adminClient := mustLogin(t, srv, admin.Username, "hunter22")

	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	target := mustCreateUser(t, uniqueName(t, "target"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &target.ID,
		ApproverID:     &target.ID,
	}
	if err := testStore.Checklists().Create(context.Background(), c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	suspendURL := srv.URL + "/admin/users/" + strconv.FormatInt(target.ID, 10) + "/active"
	resp := doForm(t, adminClient, http.MethodPost, suspendURL, url.Values{"active": {"false"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("suspend status = %d, want 200", resp.StatusCode)
	}

	got, err := testStore.Checklists().Get(context.Background(), testTenantID, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.AssignedUserID != nil {
		t.Fatalf("expected suspended user cleared as assignee, got %+v", got.AssignedUserID)
	}
	if got.ApproverID != nil {
		t.Fatalf("expected suspended user cleared as approver, got %+v", got.ApproverID)
	}
}

func TestSuspendUserAsNonAdminForbidden(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")
	other := mustCreateUser(t, uniqueName(t, "other"), "hunter22", true)

	suspendURL := srv.URL + "/admin/users/" + strconv.FormatInt(other.ID, 10) + "/active"
	resp := doForm(t, client, http.MethodPost, suspendURL, url.Values{"active": {"false"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminCannotSuspendSelf(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	suspendURL := srv.URL + "/admin/users/" + strconv.FormatInt(admin.ID, 10) + "/active"
	resp := doForm(t, client, http.MethodPost, suspendURL, url.Values{"active": {"false"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminUsersPageRendersUserList(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	resp, err := client.Get(srv.URL + "/admin/users")
	if err != nil {
		t.Fatalf("get /admin/users: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), admin.Username) {
		t.Errorf("admin users page missing %q:\n%s", admin.Username, body)
	}
}
