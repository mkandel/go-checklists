package api

import (
	"encoding/json"
	"net/http"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// registerTenantChecklistPolicyRoutes wires the admin-only checklist-creation
// restriction settings endpoints into mux.
func registerTenantChecklistPolicyRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /api/admin/tenant/checklist-policy", RequireAuth(RequireAdmin(handleGetChecklistPolicy(store))))
	mux.Handle("PUT /api/admin/tenant/checklist-policy", RequireAuth(RequireAdmin(handleUpdateChecklistPolicy(store))))
}

// checklistPolicyResponse is the read shape for a tenant's checklist-creation
// restriction settings.
type checklistPolicyResponse struct {
	Restrict       bool   `json:"restrict"`
	CreatorGroupID *int64 `json:"creator_group_id"`
}

func handleGetChecklistPolicy(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := UserFromContext(r.Context())
		tenant, err := store.Tenants().GetByID(r.Context(), actor.TenantID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, checklistPolicyResponse{
			Restrict:       tenant.RestrictChecklistCreation,
			CreatorGroupID: tenant.CreatorGroupID,
		})
	}
}

// updateChecklistPolicyRequest is the write shape for a tenant's
// checklist-creation restriction settings.
type updateChecklistPolicyRequest struct {
	Restrict       bool   `json:"restrict"`
	CreatorGroupID *int64 `json:"creator_group_id"`
}

func handleUpdateChecklistPolicy(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateChecklistPolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Restrict && req.CreatorGroupID == nil {
			http.Error(w, "creator_group_id is required when restrict is enabled", http.StatusBadRequest)
			return
		}

		actor, _ := UserFromContext(r.Context())
		policy := domain.ChecklistCreationPolicy{
			Restrict:       req.Restrict,
			CreatorGroupID: req.CreatorGroupID,
		}
		if err := store.Tenants().UpdateChecklistCreationPolicy(r.Context(), actor.TenantID, policy); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
