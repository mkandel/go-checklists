package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func decodeNotifications(t *testing.T, resp *http.Response) []domain.Notification {
	t.Helper()
	defer resp.Body.Close()
	var out []domain.Notification
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode notifications: %v", err)
	}
	return out
}

func TestListNotifications_ScopedToCaller(t *testing.T) {
	srv := newTestServer(t)
	winnerName := uniqueName(t, "winner")
	winner := mustCreateUser(t, winnerName, "hunter2", true)
	loserName := uniqueName(t, "loser")
	loser := mustCreateUser(t, loserName, "hunter2", true)
	group := mustCreateGroup(t, uniqueName(t, "team"), winner.ID, loser.ID)

	creatorClient := mustLogin(t, srv, winnerName, "hunter2")
	createResp := doJSON(t, creatorClient, http.MethodPost, srv.URL+"/checklists", map[string]any{
		"assigned_group_id": group.ID,
		"items":             []map[string]string{{"name": "Step 1"}},
	})
	created := decodeChecklist(t, createResp)
	claimURL := fmt.Sprintf("%s/checklists/%d/claim", srv.URL, created.ID)

	winnerClient := mustLogin(t, srv, winnerName, "hunter2")
	winClaim := doJSON(t, winnerClient, http.MethodPost, claimURL, nil)
	winClaim.Body.Close()

	loserClient := mustLogin(t, srv, loserName, "hunter2")
	loseClaim := doJSON(t, loserClient, http.MethodPost, claimURL, nil)
	loseClaim.Body.Close()

	listResp := doJSON(t, loserClient, http.MethodGet, srv.URL+"/notifications", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	notifications := decodeNotifications(t, listResp)
	if len(notifications) == 0 || notifications[0].Type != domain.EventClaimLost {
		t.Fatalf("expected loser to see a claim_lost notification, got %+v", notifications)
	}

	winnerListResp := doJSON(t, winnerClient, http.MethodGet, srv.URL+"/notifications", nil)
	winnerNotifications := decodeNotifications(t, winnerListResp)
	for _, n := range winnerNotifications {
		if n.ID == notifications[0].ID {
			t.Fatalf("expected winner not to see loser's notification")
		}
	}
}

func TestMarkNotificationRead_RejectsNonOwner(t *testing.T) {
	srv := newTestServer(t)
	winnerName := uniqueName(t, "winner")
	winner := mustCreateUser(t, winnerName, "hunter2", true)
	loserName := uniqueName(t, "loser")
	loser := mustCreateUser(t, loserName, "hunter2", true)
	group := mustCreateGroup(t, uniqueName(t, "team"), winner.ID, loser.ID)

	winnerClient := mustLogin(t, srv, winnerName, "hunter2")
	createResp := doJSON(t, winnerClient, http.MethodPost, srv.URL+"/checklists", map[string]any{
		"assigned_group_id": group.ID,
		"items":             []map[string]string{{"name": "Step 1"}},
	})
	created := decodeChecklist(t, createResp)
	claimURL := fmt.Sprintf("%s/checklists/%d/claim", srv.URL, created.ID)

	winClaim := doJSON(t, winnerClient, http.MethodPost, claimURL, nil)
	winClaim.Body.Close()

	loserClient := mustLogin(t, srv, loserName, "hunter2")
	loseClaim := doJSON(t, loserClient, http.MethodPost, claimURL, nil)
	loseClaim.Body.Close()

	notifications := decodeNotifications(t, doJSON(t, loserClient, http.MethodGet, srv.URL+"/notifications", nil))
	if len(notifications) == 0 {
		t.Fatal("expected at least one notification for loser")
	}
	notifID := notifications[0].ID

	readURL := fmt.Sprintf("%s/notifications/%d/read", srv.URL, notifID)

	forbiddenResp := doJSON(t, winnerClient, http.MethodPost, readURL, nil)
	defer forbiddenResp.Body.Close()
	if forbiddenResp.StatusCode != http.StatusNotFound {
		t.Fatalf("winner marking loser's notification read status = %d, want 404", forbiddenResp.StatusCode)
	}

	okResp := doJSON(t, loserClient, http.MethodPost, readURL, nil)
	defer okResp.Body.Close()
	if okResp.StatusCode != http.StatusNoContent {
		t.Fatalf("owner mark read status = %d, want 204", okResp.StatusCode)
	}
}
