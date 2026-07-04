package api

import (
	"net/http"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// handleListChecklists returns checklists relevant to the caller (creator,
// approver, direct assignee, or unclaimed-group-member), optionally narrowed
// by an exact-match ?status= query param.
func handleListChecklists(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := UserFromContext(r.Context())

		filter := domain.ChecklistFilter{TenantID: actor.TenantID, UserID: actor.ID}
		if raw := r.URL.Query().Get("status"); raw != "" {
			status := domain.ChecklistStatus(raw)
			switch status {
			case domain.StatusOpen, domain.StatusValidating, domain.StatusComplete:
				filter.Status = &status
			default:
				http.Error(w, "invalid status", http.StatusBadRequest)
				return
			}
		}

		checklists, err := store.Checklists().List(r.Context(), filter)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, checklists)
	}
}
