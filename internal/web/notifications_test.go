//go:build integration

package web_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
)

func mustCreateNotification(t *testing.T, recipientID int64, message string) *domain.Notification {
	t.Helper()
	n := &domain.Notification{
		TenantID:        testTenantID,
		RecipientUserID: recipientID,
		Type:            "test",
		Message:         message,
		EmailStatus:     domain.EmailStatusSkipped,
	}
	if err := testStore.Notifications().Create(context.Background(), n); err != nil {
		t.Fatalf("create notification: %v", err)
	}
	return n
}

func TestNotificationsListPage(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")
	n := mustCreateNotification(t, user.ID, "hello from a test")

	resp, err := client.Get(srv.URL + "/notifications")
	if err != nil {
		t.Fatalf("get /notifications: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), n.Message) {
		t.Errorf("notifications page missing message:\n%s", body)
	}
}

func TestNotificationBadgeUnreadCount(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")
	mustCreateNotification(t, user.ID, "unread one")
	mustCreateNotification(t, user.ID, "unread two")

	resp, err := client.Get(srv.URL + "/notifications/badge")
	if err != nil {
		t.Fatalf("get badge: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "2") {
		t.Errorf("badge fragment missing unread count of 2:\n%s", body)
	}
}

func TestMarkNotificationRead(t *testing.T) {
	srv := newTestServer(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")
	n := mustCreateNotification(t, user.ID, "mark me read")

	markURL := fmt.Sprintf("%s/notifications/%d/read", srv.URL, n.ID)
	resp := doForm(t, client, http.MethodPost, markURL, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if trigger := resp.Header.Get("HX-Trigger"); trigger != "notificationsRead" {
		t.Errorf("HX-Trigger = %q, want notificationsRead", trigger)
	}

	badgeResp, err := client.Get(srv.URL + "/notifications/badge")
	if err != nil {
		t.Fatalf("get badge: %v", err)
	}
	defer badgeResp.Body.Close()
	body, _ := io.ReadAll(badgeResp.Body)
	if strings.Contains(string(body), "1") {
		t.Errorf("badge still shows unread count after mark-read:\n%s", body)
	}
}

// TestNotificationStream_ReceivesLiveNotification confirms the SSE endpoint
// wakes and pushes an "event: notify" line as soon as a notification is
// created for the connected user, rather than only after the 20s poll.
func TestNotificationStream_ReceivesLiveNotification(t *testing.T) {
	srv, _ := newTestServerWithHub(t)
	user := mustCreateUser(t, uniqueName(t, "user"), "hunter22", true)
	client := mustLogin(t, srv, user.Username, "hunter22")

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/notifications/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Subscribe happens synchronously inside the handler before it writes the
	// response headers, so by the time client.Do returns, the subscription
	// is already registered — creating the notification now is guaranteed to
	// reach it.
	mustCreateNotification(t, user.ID, "live push")

	lines := make(chan string)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- line
		}
	}()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before an event: notify line arrived")
			}
			if strings.Contains(line, "event: notify") {
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for an event: notify line")
		}
	}
}

func TestMarkNotificationReadNotOwnedByUserNotFound(t *testing.T) {
	srv := newTestServer(t)
	owner := mustCreateUser(t, uniqueName(t, "owner"), "hunter22", true)
	other := mustCreateUser(t, uniqueName(t, "other"), "hunter22", true)
	n := mustCreateNotification(t, owner.ID, "not yours")

	client := mustLogin(t, srv, other.Username, "hunter22")
	markURL := fmt.Sprintf("%s/notifications/%d/read", srv.URL, n.ID)
	resp := doForm(t, client, http.MethodPost, markURL, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
