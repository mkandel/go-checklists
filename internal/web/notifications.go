package web

import (
	"net/http"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	notificationsTemplate      = pageTemplate("notifications_list.html", "partials/notifications_table.html")
	notificationsTableFragment = fragmentTemplate("partials/notifications_table.html")
	notificationBadgeFragment  = fragmentTemplate("partials/notification_badge.html")
)

// registerNotificationRoutes wires the notifications page, its polling
// unread-count badge, and the mark-read fragment.
func registerNotificationRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /notifications", requireAuthPage(handleNotificationsPage(store)))
	mux.Handle("GET /notifications/badge", api.RequireAuth(handleNotificationBadgeFragment(store)))
	mux.Handle("POST /notifications/{id}/read", api.RequireAuth(handleMarkNotificationReadFragment(store)))
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
