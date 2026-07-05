//go:build integration

package web_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func createChecklistUI(t *testing.T, client *http.Client, srvURL string, req map[string]any) int64 {
	t.Helper()
	resp := doJSON(t, client, http.MethodPost, srvURL+"/checklists", req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create checklist: status = %d, want 201: %s", resp.StatusCode, body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created checklist: %v", err)
	}
	return created.ID
}

func TestChecklistDetailPageRenders(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "First item")
	client := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, client, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
	})

	resp, err := client.Get(fmt.Sprintf("%s/checklists/%d", srv.URL, id))
	if err != nil {
		t.Fatalf("get detail page: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "First item") {
		t.Errorf("detail page missing item name:\n%s", body)
	}
}

func TestClaimChecklist(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	claimer := mustCreateUser(t, uniqueName(t, "claimer"), "hunter22", true)
	group := mustCreateGroup(t, uniqueName(t, "Ops"), claimer.ID)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item")
	creatorClient := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, creatorClient, srv.URL, map[string]any{
		"template_id":       tmpl.ID,
		"assigned_group_id": group.ID,
	})

	claimerClient := mustLogin(t, srv, claimer.Username, "hunter22")
	claimURL := fmt.Sprintf("%s/checklists/%d/claim", srv.URL, id)
	resp := doForm(t, claimerClient, http.MethodPost, claimURL, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), `class="error"`) && strings.Contains(string(body), "claimed") {
		t.Errorf("unexpected claim error in panel:\n%s", body)
	}
}

func TestCheckItemAsAssigneeCompletesChecklist(t *testing.T) {
	srv := newTestServer(t)
	assignee := mustCreateUser(t, uniqueName(t, "assignee"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Only item")
	client := mustLogin(t, srv, assignee.Username, "hunter22")

	id := createChecklistUI(t, client, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": assignee.ID,
	})

	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	itemID := c.Items[0].ID

	checkURL := fmt.Sprintf("%s/checklists/%d/items/%d/check", srv.URL, id, itemID)
	resp := doForm(t, client, http.MethodPost, checkURL, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "complete") {
		t.Errorf("panel doesn't show complete status after checking the only item:\n%s", body)
	}
}

func TestCheckItemAsNonAssigneeShowsFlashErrorWith200(t *testing.T) {
	srv := newTestServer(t)
	assignee := mustCreateUser(t, uniqueName(t, "assignee"), "hunter22", true)
	other := mustCreateUser(t, uniqueName(t, "other"), "hunter22", true)
	assigneeClient := mustLogin(t, srv, assignee.Username, "hunter22")
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item")

	id := createChecklistUI(t, assigneeClient, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": assignee.ID,
	})
	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	itemID := c.Items[0].ID

	otherClient := mustLogin(t, srv, other.Username, "hunter22")
	checkURL := fmt.Sprintf("%s/checklists/%d/items/%d/check", srv.URL, id, itemID)
	resp := doForm(t, otherClient, http.MethodPost, checkURL, nil)
	defer resp.Body.Close()
	// Domain-rule violations render inline with HTTP 200 -- htmx doesn't swap
	// non-2xx responses by default (see withChecklistMutationUI).
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not the responsible assignee") {
		t.Errorf("panel missing FlashError for non-assignee check attempt:\n%s", body)
	}
}

func TestCheckItemUnknownItemNotFound(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item")
	client := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, client, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
	})

	checkURL := fmt.Sprintf("%s/checklists/%d/items/999999999/check", srv.URL, id)
	resp := doForm(t, client, http.MethodPost, checkURL, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSetItemCheckedAsCreatorOverride(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	assignee := mustCreateUser(t, uniqueName(t, "assignee"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item")
	creatorClient := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, creatorClient, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": assignee.ID,
	})
	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	itemID := c.Items[0].ID

	setURL := fmt.Sprintf("%s/checklists/%d/items/%d/checked", srv.URL, id, itemID)
	resp := doForm(t, creatorClient, http.MethodPut, setURL, url.Values{"checked": {"true"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "checked-label") {
		t.Errorf("panel doesn't show item as checked after creator override:\n%s", body)
	}
}

func TestSetItemCheckedAsNonCreatorForbiddenFlashError(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	assignee := mustCreateUser(t, uniqueName(t, "assignee"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item")
	creatorClient := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, creatorClient, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": assignee.ID,
	})
	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	itemID := c.Items[0].ID

	assigneeClient := mustLogin(t, srv, assignee.Username, "hunter22")
	setURL := fmt.Sprintf("%s/checklists/%d/items/%d/checked", srv.URL, id, itemID)
	resp := doForm(t, assigneeClient, http.MethodPut, setURL, url.Values{"checked": {"true"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not this checklist&#39;s creator") {
		t.Errorf("panel missing FlashError for non-creator override attempt:\n%s", body)
	}
}

func TestApproveRejectFlow(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	assignee := mustCreateUser(t, uniqueName(t, "assignee"), "hunter22", true)
	approver := mustCreateUser(t, uniqueName(t, "approver"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item")
	creatorClient := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, creatorClient, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": assignee.ID,
		"approver_id":      approver.ID,
	})
	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	itemID := c.Items[0].ID

	assigneeClient := mustLogin(t, srv, assignee.Username, "hunter22")
	checkResp := doForm(t, assigneeClient, http.MethodPost, fmt.Sprintf("%s/checklists/%d/items/%d/check", srv.URL, id, itemID), nil)
	checkBody, _ := io.ReadAll(checkResp.Body)
	checkResp.Body.Close()
	if !strings.Contains(string(checkBody), "validating") {
		t.Fatalf("panel doesn't show validating status after checking last item:\n%s", checkBody)
	}

	approverClient := mustLogin(t, srv, approver.Username, "hunter22")
	rejectURL := fmt.Sprintf("%s/checklists/%d/reject", srv.URL, id)
	rejectForm := url.Values{}
	rejectForm.Add("item_id", fmt.Sprint(itemID))
	rejectResp := doForm(t, approverClient, http.MethodPost, rejectURL, rejectForm)
	defer rejectResp.Body.Close()
	if rejectResp.StatusCode != http.StatusOK {
		t.Fatalf("reject status = %d, want 200", rejectResp.StatusCode)
	}
	rejectBody, _ := io.ReadAll(rejectResp.Body)
	if !strings.Contains(string(rejectBody), "open") {
		t.Errorf("panel doesn't show open status after reject:\n%s", rejectBody)
	}

	recheckResp := doForm(t, assigneeClient, http.MethodPost, fmt.Sprintf("%s/checklists/%d/items/%d/check", srv.URL, id, itemID), nil)
	recheckResp.Body.Close()

	approveResp := doForm(t, approverClient, http.MethodPost, fmt.Sprintf("%s/checklists/%d/approve", srv.URL, id), nil)
	defer approveResp.Body.Close()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d, want 200", approveResp.StatusCode)
	}
	approveBody, _ := io.ReadAll(approveResp.Body)
	if !strings.Contains(string(approveBody), "complete") {
		t.Errorf("panel doesn't show complete status after approve:\n%s", approveBody)
	}
}

func TestApproveByNonApproverFlashError(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	approver := mustCreateUser(t, uniqueName(t, "approver"), "hunter22", true)
	outsider := mustCreateUser(t, uniqueName(t, "outsider"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item")
	creatorClient := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, creatorClient, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
		"approver_id":      approver.ID,
	})
	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	itemID := c.Items[0].ID
	checkResp := doForm(t, creatorClient, http.MethodPost, fmt.Sprintf("%s/checklists/%d/items/%d/check", srv.URL, id, itemID), nil)
	checkResp.Body.Close()

	outsiderClient := mustLogin(t, srv, outsider.Username, "hunter22")
	resp := doForm(t, outsiderClient, http.MethodPost, fmt.Sprintf("%s/checklists/%d/approve", srv.URL, id), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not the checklist&#39;s approver") {
		t.Errorf("panel missing FlashError for non-approver approve attempt:\n%s", body)
	}
}

func TestAddAndRemoveItemAsCreator(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "First")
	client := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, client, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
	})

	addResp := doForm(t, client, http.MethodPost, fmt.Sprintf("%s/checklists/%d/items", srv.URL, id), url.Values{"name": {"Second"}, "validation_ref": {"ref"}})
	defer addResp.Body.Close()
	if addResp.StatusCode != http.StatusOK {
		t.Fatalf("add item status = %d, want 200", addResp.StatusCode)
	}
	addBody, _ := io.ReadAll(addResp.Body)
	if !strings.Contains(string(addBody), "Second") {
		t.Errorf("panel missing newly added item:\n%s", addBody)
	}

	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	var secondItemID int64
	for _, it := range c.Items {
		if it.Name == "Second" {
			secondItemID = it.ID
		}
	}
	if secondItemID == 0 {
		t.Fatal("could not find newly added item")
	}

	removeReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/checklists/%d/items/%d", srv.URL, id, secondItemID), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token := csrfTokenFromJar(t, client, removeReq.URL.String()); token != "" {
		removeReq.Header.Set("X-CSRF-Token", token)
	}
	removeResp, err := client.Do(removeReq)
	if err != nil {
		t.Fatalf("remove item: %v", err)
	}
	defer removeResp.Body.Close()
	if removeResp.StatusCode != http.StatusOK {
		t.Fatalf("remove item status = %d, want 200", removeResp.StatusCode)
	}
	removeBody, _ := io.ReadAll(removeResp.Body)
	if strings.Contains(string(removeBody), "Second") {
		t.Errorf("panel still shows removed item:\n%s", removeBody)
	}
}

func TestReorderItemsAsCreator(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "First", "Second")
	client := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, client, srv.URL, map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
	})
	c, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if len(c.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(c.Items))
	}
	reversed := []int64{c.Items[1].ID, c.Items[0].ID}

	resp := doJSON(t, client, http.MethodPut, fmt.Sprintf("%s/checklists/%d/items/order", srv.URL, id), map[string]any{"item_ids": reversed})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	after, err := testStore.Checklists().Get(context.Background(), testTenantID, id)
	if err != nil {
		t.Fatalf("get checklist after reorder: %v", err)
	}
	if after.Items[0].ID != reversed[0] || after.Items[1].ID != reversed[1] {
		t.Errorf("items not reordered as expected: got %+v", after.Items)
	}
}

func TestChecklistDetailHiddenNotVisibleReturns404(t *testing.T) {
	srv := newTestServer(t)
	creator := mustCreateUser(t, uniqueName(t, "creator"), "hunter22", true)
	outsider := mustCreateUser(t, uniqueName(t, "outsider"), "hunter22", true)
	group := mustCreateGroup(t, uniqueName(t, "Ops"))
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Secret item")
	creatorClient := mustLogin(t, srv, creator.Username, "hunter22")

	id := createChecklistUI(t, creatorClient, srv.URL, map[string]any{
		"template_id":       tmpl.ID,
		"assigned_group_id": group.ID,
		"hidden":            true,
	})

	outsiderClient := mustLogin(t, srv, outsider.Username, "hunter22")
	resp, err := outsiderClient.Get(fmt.Sprintf("%s/checklists/%d", srv.URL, id))
	if err != nil {
		t.Fatalf("get detail page: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
