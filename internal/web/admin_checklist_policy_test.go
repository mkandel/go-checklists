//go:build integration

package web_test

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestUpdateChecklistPolicyEnableWithGroup(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")
	group := mustCreateGroup(t, uniqueName(t, "Creators"))

	form := url.Values{"restrict": {"true"}, "creator_group_id": {strconv.FormatInt(group.ID, 10)}}
	resp := doForm(t, client, http.MethodPut, srv.URL+"/admin/checklist-policy", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Saved") {
		t.Errorf("policy fragment missing save confirmation:\n%s", body)
	}
	if !strings.Contains(string(body), `selected`) {
		t.Errorf("policy fragment missing selected group option:\n%s", body)
	}
	if !strings.Contains(string(body), "restrict: true") {
		t.Errorf("policy fragment x-data missing restrict: true:\n%s", body)
	}
}

func TestUpdateChecklistPolicyEnableWithoutGroupShowsError(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")

	form := url.Values{"restrict": {"true"}}
	resp := doForm(t, client, http.MethodPut, srv.URL+"/admin/checklist-policy", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "creator group is required") {
		t.Errorf("policy fragment missing validation error:\n%s", body)
	}
}

func TestUpdateChecklistPolicyDisable(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	client := mustLogin(t, srv, admin.Username, "hunter22")
	group := mustCreateGroup(t, uniqueName(t, "Creators"))

	enableForm := url.Values{"restrict": {"true"}, "creator_group_id": {strconv.FormatInt(group.ID, 10)}}
	enableResp := doForm(t, client, http.MethodPut, srv.URL+"/admin/checklist-policy", enableForm)
	enableResp.Body.Close()

	disableResp := doForm(t, client, http.MethodPut, srv.URL+"/admin/checklist-policy", url.Values{})
	defer disableResp.Body.Close()
	if disableResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", disableResp.StatusCode)
	}
	body, _ := io.ReadAll(disableResp.Body)
	if !strings.Contains(string(body), "restrict: false") {
		t.Errorf("policy fragment x-data missing restrict: false after disable:\n%s", body)
	}
}

func TestChecklistPolicyPageAndUpdateAsNonAdminForbidden(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	getResp, err := client.Get(srv.URL + "/admin/checklist-policy")
	if err != nil {
		t.Fatalf("get policy page: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET status = %d, want 403", getResp.StatusCode)
	}

	putResp := doForm(t, client, http.MethodPut, srv.URL+"/admin/checklist-policy", url.Values{"restrict": {"true"}})
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusForbidden {
		t.Fatalf("PUT status = %d, want 403", putResp.StatusCode)
	}
}
