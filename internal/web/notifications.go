package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/notify"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// sseKeepaliveInterval is how often handleNotificationStream writes a
// no-op comment line to keep the connection alive through idle-connection
// timeouts on proxies/load balancers in front of the app.
const sseKeepaliveInterval = 25 * time.Second

var (
	notificationsTemplate      = pageTemplate("notifications_list.html", "partials/notifications_table.html")
	notificationsTableFragment = fragmentTemplate("partials/notifications_table.html")
	notificationBadgeFragment  = fragmentTemplate("partials/notification_badge.html")
)

// registerNotificationRoutes wires the notifications page, its polling
// unread-count badge, the mark-read fragment, and the SSE stream that pushes
// a live wake-up on top of the poll.
func registerNotificationRoutes(mux *http.ServeMux, store *postgres.Store, hub *notify.Hub) {
	mux.HandleFunc("GET /notifications", requireAuthPage(handleNotificationsPage(store)))
	mux.Handle("GET /notifications/badge", api.RequireAuth(handleNotificationBadgeFragment(store)))
	mux.Handle("POST /notifications/{id}/read", api.RequireAuth(handleMarkNotificationReadFragment(store)))
	mux.Handle("GET /notifications/stream", api.RequireAuth(handleNotificationStream(hub)))
}

type notificationsPageData struct {
	baseData
	Notifications []domain.Notification
}

func handleNotificationsPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		notifications, err := store.Notifications().ListForUser(r.Context(), actor.TenantID, actor.ID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderPage(w, notificationsTemplate, notificationsPageData{
			baseData:      baseData{Actor: actor},
			Notifications: notifications,
		})
	}
}

// renderNotificationsTable re-renders the notifications list fragment and
// sets HX-Trigger so the nav badge (listening for notificationsRead
// from:body) refreshes its unread count.
func renderNotificationsTable(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User) {
	notifications, err := store.Notifications().ListForUser(r.Context(), actor.TenantID, actor.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", "notificationsRead")
	renderFragment(w, notificationsTableFragment, "notifications_table", notificationsPageData{Notifications: notifications})
}

func handleNotificationBadgeFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		notifications, err := store.Notifications().ListForUser(r.Context(), actor.TenantID, actor.ID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		unread := 0
		for _, n := range notifications {
			if n.ReadAt == nil {
				unread++
			}
		}
		renderFragment(w, notificationBadgeFragment, "notification_badge", struct{ Count int }{Count: unread})
	}
}

// handleNotificationStream is a Server-Sent Events endpoint: it holds the
// connection open and writes a "notify" event each time hub wakes this
// user's (tenantID, userID), so the client can refresh the badge instantly
// instead of waiting for its next poll. This is additive — the badge's
// existing 20s poll stays as a fallback if SSE never reaches a client (older
// browser, a proxy that kills long-lived connections).
func handleNotificationStream(hub *notify.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		ch, unsubscribe := hub.Subscribe(actor.TenantID, actor.ID)
		defer unsubscribe()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		keepalive := time.NewTicker(sseKeepaliveInterval)
		defer keepalive.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				fmt.Fprint(w, "event: notify\ndata: {}\n\n")
				flusher.Flush()
			case <-keepalive.C:
				fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}
}

func handleMarkNotificationReadFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid notification id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		if err := store.Notifications().MarkRead(r.Context(), actor.TenantID, id, actor.ID); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		renderNotificationsTable(w, r, store, actor)
	}
}
