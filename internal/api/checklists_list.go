package api

import (
	"net/http"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// checklistListSortColumns allowlists the ?sort= values accepted from
// GET /api/checklists — kept in sync with checklistSortColumns in
// internal/store/postgres/checklists.go.
var checklistListSortColumns = map[string]bool{"name": true, "status": true, "created_at": true}

// handleListChecklists returns checklists relevant to the caller (creator,
// approver, direct assignee, or unclaimed-group-member), optionally narrowed
// by an exact-match ?status= query param and ordered by ?sort=/?dir=.
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
		if sort := r.URL.Query().Get("sort"); checklistListSortColumns[sort] {
			filter.SortBy = sort
		}
		if r.URL.Query().Get("dir") == "asc" {
			filter.SortDir = "asc"
		}

		checklists, err := store.Checklists().List(r.Context(), filter)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, checklists)
	}
}
