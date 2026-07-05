//go:build integration

package web_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func TestChecklistsListPageAndFilter(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	req := map[string]any{
		"items":            []map[string]string{{"name": "Step one", "validation_ref": ""}},
		"assigned_user_id": user.ID,
	}
	createResp := doJSON(t, client, http.MethodPost, srv.URL+"/checklists", req)
	createResp.Body.Close()

	resp, err := client.Get(srv.URL + "/checklists")
	if err != nil {
		t.Fatalf("get /checklists: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "New checklist") {
		t.Errorf("list page missing New checklist link for unrestricted user:\n%s", body)
	}

	completeResp, err := client.Get(srv.URL + "/checklists?status=complete")
	if err != nil {
		t.Fatalf("get filtered list: %v", err)
	}
	defer completeResp.Body.Close()
	completeBody, _ := io.ReadAll(completeResp.Body)
	if strings.Contains(string(completeBody), "Step one") {
		t.Errorf("complete filter unexpectedly shows an open checklist:\n%s", completeBody)
	}
}

func TestChecklistsNewPageAndCreate(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	newPageResp, err := client.Get(srv.URL + "/checklists/new")
	if err != nil {
		t.Fatalf("get /checklists/new: %v", err)
	}
	defer newPageResp.Body.Close()
	if newPageResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", newPageResp.StatusCode)
	}

	req := map[string]any{
		"items":            []map[string]string{{"name": "Do the thing", "validation_ref": "ref-1"}},
		"assigned_user_id": user.ID,
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/checklists", req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201: %s", resp.StatusCode, body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero checklist id")
	}
}

func TestChecklistsCreateInvalidAssignmentBadRequest(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	req := map[string]any{
		"items": []map[string]string{{"name": "Orphan item", "validation_ref": ""}},
	}
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/checklists", req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// --- Creator-vs-user checklist creation restriction (task #78; web-layer
// coverage deferred to this file per the approved plan) ---

func enableChecklistCreationRestriction(t *testing.T, adminClient *http.Client, srvURL string, groupID int64) {
	t.Helper()
	form := url.Values{"restrict": {"true"}, "creator_group_id": {strconv.FormatInt(groupID, 10)}}
	resp := doForm(t, adminClient, http.MethodPut, srvURL+"/admin/checklist-policy", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable restriction: status = %d, want 200", resp.StatusCode)
	}
}

func TestChecklistCreationRestrictionBlocksNonMember(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	outsider := mustCreateUser(t, uniqueName(t, "outsider"), "hunter22", true)
	group := mustCreateGroup(t, uniqueName(t, "Creators"))

	adminClient := mustLogin(t, srv, admin.Username, "hunter22")
	enableChecklistCreationRestriction(t, adminClient, srv.URL, group.ID)

	outsiderClient := mustLogin(t, srv, outsider.Username, "hunter22")

	newPageResp, err := outsiderClient.Get(srv.URL + "/checklists/new")
	if err != nil {
		t.Fatalf("get /checklists/new: %v", err)
	}
	defer newPageResp.Body.Close()
	if newPageResp.StatusCode != http.StatusForbidden {
		t.Fatalf("new page status = %d, want 403", newPageResp.StatusCode)
	}

	listResp, err := outsiderClient.Get(srv.URL + "/checklists")
	if err != nil {
		t.Fatalf("get /checklists: %v", err)
	}
	defer listResp.Body.Close()
	listBody, _ := io.ReadAll(listResp.Body)
	if strings.Contains(string(listBody), "New checklist") {
		t.Errorf("list page shows New checklist link for a restricted outsider:\n%s", listBody)
	}

	req := map[string]any{
		"items":            []map[string]string{{"name": "Blocked item", "validation_ref": ""}},
		"assigned_user_id": outsider.ID,
	}
	createResp := doJSON(t, outsiderClient, http.MethodPost, srv.URL+"/checklists", req)
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusForbidden {
		t.Fatalf("create status = %d, want 403", createResp.StatusCode)
	}
	body, _ := io.ReadAll(createResp.Body)
	if !strings.Contains(string(body), domain.ErrChecklistCreationRestricted.Error()) {
		t.Errorf("create response missing restriction error text:\n%s", body)
	}
}

func TestChecklistCreationRestrictionAllowsMemberAndAdmin(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	member := mustCreateUser(t, uniqueName(t, "member"), "hunter22", true)
	group := mustCreateGroup(t, uniqueName(t, "Creators"), member.ID)

	adminClient := mustLogin(t, srv, admin.Username, "hunter22")
	enableChecklistCreationRestriction(t, adminClient, srv.URL, group.ID)

	adminReq := map[string]any{
		"items":            []map[string]string{{"name": "Admin item", "validation_ref": ""}},
		"assigned_user_id": admin.ID,
	}
	adminResp := doJSON(t, adminClient, http.MethodPost, srv.URL+"/checklists", adminReq)
	defer adminResp.Body.Close()
	if adminResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(adminResp.Body)
		t.Fatalf("admin create status = %d, want 201: %s", adminResp.StatusCode, body)
	}

	memberClient := mustLogin(t, srv, member.Username, "hunter22")
	memberReq := map[string]any{
		"items":            []map[string]string{{"name": "Member item", "validation_ref": ""}},
		"assigned_user_id": member.ID,
	}
	memberResp := doJSON(t, memberClient, http.MethodPost, srv.URL+"/checklists", memberReq)
	defer memberResp.Body.Close()
	if memberResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(memberResp.Body)
		t.Fatalf("member create status = %d, want 201: %s", memberResp.StatusCode, body)
	}
}

func TestChecklistCreationRestrictionOffRestoresDefault(t *testing.T) {
	srv := newTestServer(t)
	admin := mustCreateAdminUser(t, uniqueName(t, "admin"), "hunter22")
	outsider := mustCreateUser(t, uniqueName(t, "outsider"), "hunter22", true)
	group := mustCreateGroup(t, uniqueName(t, "Creators"))

	adminClient := mustLogin(t, srv, admin.Username, "hunter22")
	enableChecklistCreationRestriction(t, adminClient, srv.URL, group.ID)

	disableResp := doForm(t, adminClient, http.MethodPut, srv.URL+"/admin/checklist-policy", url.Values{})
	disableResp.Body.Close()

	outsiderClient := mustLogin(t, srv, outsider.Username, "hunter22")
	req := map[string]any{
		"items":            []map[string]string{{"name": "Now allowed", "validation_ref": ""}},
		"assigned_user_id": outsider.ID,
	}
	resp := doJSON(t, outsiderClient, http.MethodPost, srv.URL+"/checklists", req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 201: %s", resp.StatusCode, body)
	}
}
