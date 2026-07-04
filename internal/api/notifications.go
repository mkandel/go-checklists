package api

import (
	"net/http"

	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// registerNotificationRoutes wires the notification endpoints into mux, all
// gated behind RequireAuth.
func registerNotificationRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /notifications", RequireAuth(handleListNotifications(store)))
	mux.Handle("POST /notifications/{id}/read", RequireAuth(handleMarkNotificationRead(store)))
}

func handleListNotifications(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := UserFromContext(r.Context())

		notifications, err := store.Notifications().ListForUser(r.Context(), actor.TenantID, actor.ID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, notifications)
	}
}

func handleMarkNotificationRead(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid notification id", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		if err := store.Notifications().MarkRead(r.Context(), actor.TenantID, id, actor.ID); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
