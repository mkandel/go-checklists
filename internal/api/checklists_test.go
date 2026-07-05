//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

// csrfCookieName mirrors the unexported constant of the same name in
// internal/api — the test package can't import it directly.
const csrfCookieName = "checklists_csrf"

func mustLogin(t *testing.T, srv *httptest.Server, username, password string) *http.Client {
	t.Helper()
	client := newClient(t)
	resp := doLogin(t, client, srv.URL, username, password)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login failed: status = %d", resp.StatusCode)
	}
	return client
}

func mustCreateGroup(t *testing.T, name string, memberIDs ...int64) *domain.Group {
	t.Helper()
	g := &domain.Group{TenantID: testTenantID, Name: name}
	if err := testStore.Groups().Create(context.Background(), g); err != nil {
		t.Fatalf("create group: %v", err)
	}
	for _, uid := range memberIDs {
		if err := testStore.Groups().AddMember(context.Background(), testTenantID, g.ID, uid); err != nil {
			t.Fatalf("add group member: %v", err)
		}
	}
	return g
}

// doJSON sends a JSON request, automatically attaching the CSRF header from
// client's cookie jar (mirroring what a real browser-side script would do).
// Use doJSONNoCSRF for tests that need to omit or corrupt that header.
func doJSON(t *testing.T, client *http.Client, method, reqURL string, body any) *http.Response {
	t.Helper()
	return doJSONOpts(t, client, method, reqURL, body, true)
}

// doJSONNoCSRF is doJSON without the automatic X-CSRF-Token header, for
// tests that need to exercise the CSRF check itself.
func doJSONNoCSRF(t *testing.T, client *http.Client, method, reqURL string, body any) *http.Response {
	t.Helper()
	return doJSONOpts(t, client, method, reqURL, body, false)
}

func doJSONOpts(t *testing.T, client *http.Client, method, reqURL string, body any, attachCSRF bool) *http.Response {
	t.Helper()
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = b
	}
	req, err := http.NewRequest(method, reqURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if attachCSRF {
		if token := csrfTokenFromJar(t, client, reqURL); token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, reqURL, err)
	}
	return resp
}

// csrfTokenFromJar reads the CSRF cookie client would send for reqURL, so
// doJSON can echo it in the X-CSRF-Token header the way browser-side JS
// would.
func csrfTokenFromJar(t *testing.T, client *http.Client, reqURL string) string {
	t.Helper()
	u, err := url.Parse(reqURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == csrfCookieName {
			return c.Value
		}
	}
	return ""
}

func decodeChecklist(t *testing.T, resp *http.Response) *domain.Checklist {
	t.Helper()
	defer resp.Body.Close()
	var c domain.Checklist
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatalf("decode checklist: %v", err)
	}
	return &c
}

func TestCreateChecklist_FromTemplate(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1", "Step 2")
	client := mustLogin(t, srv, creatorName, "hunter2")

	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	created := decodeChecklist(t, resp)
	if created.ID == 0 {
		t.Fatal("expected non-zero checklist id")
	}
	if len(created.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(created.Items))
	}
	if created.CreatorID != creator.ID {
		t.Fatalf("CreatorID = %d, want %d", created.CreatorID, creator.ID)
	}

	getResp := doJSON(t, client, http.MethodGet, fmt.Sprintf("%s/api/checklists/%d", srv.URL, created.ID), nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}
	got := decodeChecklist(t, getResp)
	if got.ID != created.ID {
		t.Fatalf("ID = %d, want %d", got.ID, created.ID)
	}
}

func TestCreateChecklist_RequiresAssignment(t *testing.T) {
	srv := newTestServer(t)
	username := uniqueName(t, "alice")
	mustCreateUser(t, username, "hunter2", true)
	client := mustLogin(t, srv, username, "hunter2")

	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/checklists", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGetChecklist_HiddenVisibility(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	outsiderName := uniqueName(t, "outsider")
	mustCreateUser(t, outsiderName, "hunter2", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1")

	creatorClient := mustLogin(t, srv, creatorName, "hunter2")
	outsiderClient := mustLogin(t, srv, outsiderName, "hunter2")

	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
		"hidden":           true,
	})
	created := decodeChecklist(t, createResp)
	getURL := fmt.Sprintf("%s/api/checklists/%d", srv.URL, created.ID)

	ownResp := doJSON(t, creatorClient, http.MethodGet, getURL, nil)
	defer ownResp.Body.Close()
	if ownResp.StatusCode != http.StatusOK {
		t.Fatalf("creator get status = %d, want 200", ownResp.StatusCode)
	}

	outsiderResp := doJSON(t, outsiderClient, http.MethodGet, getURL, nil)
	defer outsiderResp.Body.Close()
	if outsiderResp.StatusCode != http.StatusNotFound {
		t.Fatalf("outsider get status = %d, want 404", outsiderResp.StatusCode)
	}
}

func TestClaimChecklist_HappyPathAndLostRace(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	mustCreateUser(t, creatorName, "hunter2", true)
	aliceName := uniqueName(t, "alice")
	alice := mustCreateUser(t, aliceName, "hunter2", true)
	bobName := uniqueName(t, "bob")
	bob := mustCreateUser(t, bobName, "hunter2", true)
	group := mustCreateGroup(t, uniqueName(t, "group"), alice.ID, bob.ID)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1")

	creatorClient := mustLogin(t, srv, creatorName, "hunter2")
	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"template_id":       tmpl.ID,
		"assigned_group_id": group.ID,
	})
	created := decodeChecklist(t, createResp)
	if created.AssignedUserID != nil {
		t.Fatal("expected unclaimed checklist")
	}
	claimURL := fmt.Sprintf("%s/api/checklists/%d/claim", srv.URL, created.ID)

	aliceClient := mustLogin(t, srv, aliceName, "hunter2")
	aliceClaimResp := doJSON(t, aliceClient, http.MethodPost, claimURL, nil)
	defer aliceClaimResp.Body.Close()
	if aliceClaimResp.StatusCode != http.StatusNoContent {
		t.Fatalf("alice claim status = %d, want 204", aliceClaimResp.StatusCode)
	}

	bobClient := mustLogin(t, srv, bobName, "hunter2")
	bobClaimResp := doJSON(t, bobClient, http.MethodPost, claimURL, nil)
	defer bobClaimResp.Body.Close()
	if bobClaimResp.StatusCode != http.StatusConflict {
		t.Fatalf("bob claim status = %d, want 409", bobClaimResp.StatusCode)
	}

	getResp := doJSON(t, creatorClient, http.MethodGet, fmt.Sprintf("%s/api/checklists/%d", srv.URL, created.ID), nil)
	got := decodeChecklist(t, getResp)
	if got.AssignedUserID == nil || *got.AssignedUserID != alice.ID {
		t.Fatalf("expected assigned to alice (%d), got %v", alice.ID, got.AssignedUserID)
	}
}

func TestCheckItem_HappyPathAndForbidden(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	otherName := uniqueName(t, "other")
	mustCreateUser(t, otherName, "hunter2", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1")

	creatorClient := mustLogin(t, srv, creatorName, "hunter2")
	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
	})
	created := decodeChecklist(t, createResp)
	itemID := created.Items[0].ID
	checkURL := fmt.Sprintf("%s/api/checklists/%d/items/%d/check", srv.URL, created.ID, itemID)

	otherClient := mustLogin(t, srv, otherName, "hunter2")
	forbiddenResp := doJSON(t, otherClient, http.MethodPost, checkURL, nil)
	defer forbiddenResp.Body.Close()
	if forbiddenResp.StatusCode != http.StatusForbidden {
		t.Fatalf("other-user check status = %d, want 403", forbiddenResp.StatusCode)
	}

	okResp := doJSON(t, creatorClient, http.MethodPost, checkURL, nil)
	if okResp.StatusCode != http.StatusOK {
		t.Fatalf("check status = %d, want 200", okResp.StatusCode)
	}
	updated := decodeChecklist(t, okResp)
	if !updated.Items[0].Checked {
		t.Fatal("expected item checked")
	}
	if updated.Status != domain.StatusComplete {
		t.Fatalf("expected complete (single item, no approver), got %s", updated.Status)
	}
}

func TestApproveFlow(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	approverName := uniqueName(t, "approver")
	approver := mustCreateUser(t, approverName, "hunter2", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1")

	creatorClient := mustLogin(t, srv, creatorName, "hunter2")
	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
		"approver_id":      approver.ID,
	})
	created := decodeChecklist(t, createResp)
	itemID := created.Items[0].ID

	checkResp := doJSON(t, creatorClient, http.MethodPost,
		fmt.Sprintf("%s/api/checklists/%d/items/%d/check", srv.URL, created.ID, itemID), nil)
	checked := decodeChecklist(t, checkResp)
	if checked.Status != domain.StatusValidating {
		t.Fatalf("expected validating, got %s", checked.Status)
	}

	approveURL := fmt.Sprintf("%s/api/checklists/%d/approve", srv.URL, created.ID)

	forbiddenResp := doJSON(t, creatorClient, http.MethodPost, approveURL, nil)
	defer forbiddenResp.Body.Close()
	if forbiddenResp.StatusCode != http.StatusForbidden {
		t.Fatalf("creator approve status = %d, want 403", forbiddenResp.StatusCode)
	}

	approverClient := mustLogin(t, srv, approverName, "hunter2")
	approveResp := doJSON(t, approverClient, http.MethodPost, approveURL, nil)
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d, want 200", approveResp.StatusCode)
	}
	final := decodeChecklist(t, approveResp)
	if final.Status != domain.StatusComplete {
		t.Fatalf("expected complete, got %s", final.Status)
	}
}

func TestRejectFlow(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	approverName := uniqueName(t, "approver")
	approver := mustCreateUser(t, approverName, "hunter2", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1")

	creatorClient := mustLogin(t, srv, creatorName, "hunter2")
	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
		"approver_id":      approver.ID,
	})
	created := decodeChecklist(t, createResp)
	itemID := created.Items[0].ID

	checkResp := doJSON(t, creatorClient, http.MethodPost,
		fmt.Sprintf("%s/api/checklists/%d/items/%d/check", srv.URL, created.ID, itemID), nil)
	checkResp.Body.Close()

	approverClient := mustLogin(t, srv, approverName, "hunter2")
	rejectResp := doJSON(t, approverClient, http.MethodPost,
		fmt.Sprintf("%s/api/checklists/%d/reject", srv.URL, created.ID),
		map[string]any{"item_ids": []int64{itemID}})
	if rejectResp.StatusCode != http.StatusOK {
		t.Fatalf("reject status = %d, want 200", rejectResp.StatusCode)
	}
	rejected := decodeChecklist(t, rejectResp)
	if rejected.Status != domain.StatusOpen {
		t.Fatalf("expected open after reject, got %s", rejected.Status)
	}
	if rejected.Items[0].Checked {
		t.Fatal("expected item unchecked after reject")
	}
	if rejected.Items[0].AssigneeOverrideUserID == nil || *rejected.Items[0].AssigneeOverrideUserID != creator.ID {
		t.Fatalf("expected override to original checker %d, got %v", creator.ID, rejected.Items[0].AssigneeOverrideUserID)
	}
}

func TestAddItem_CreatorOverride_ForcesReopen(t *testing.T) {
	srv := newTestServer(t)
	creatorName := uniqueName(t, "creator")
	creator := mustCreateUser(t, creatorName, "hunter2", true)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1")

	creatorClient := mustLogin(t, srv, creatorName, "hunter2")
	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/api/checklists", map[string]any{
		"template_id":      tmpl.ID,
		"assigned_user_id": creator.ID,
	})
	created := decodeChecklist(t, createResp)
	itemID := created.Items[0].ID

	checkResp := doJSON(t, creatorClient, http.MethodPost,
		fmt.Sprintf("%s/api/checklists/%d/items/%d/check", srv.URL, created.ID, itemID), nil)
	checked := decodeChecklist(t, checkResp)
	if checked.Status != domain.StatusComplete {
		t.Fatalf("expected complete, got %s", checked.Status)
	}

	addResp := doJSON(t, creatorClient, http.MethodPost,
		fmt.Sprintf("%s/api/checklists/%d/items", srv.URL, created.ID),
		map[string]any{"name": "Extra step"})
	if addResp.StatusCode != http.StatusOK {
		t.Fatalf("add item status = %d, want 200", addResp.StatusCode)
	}
	updated := decodeChecklist(t, addResp)
	if updated.Status != domain.StatusOpen {
		t.Fatalf("expected forced back to open, got %s", updated.Status)
	}
	if len(updated.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(updated.Items))
	}
}
