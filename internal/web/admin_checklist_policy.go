package web

import (
	"net/http"
	"strconv"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	checklistPolicyTemplate = pageTemplate("admin_checklist_policy.html", "partials/checklist_policy_form.html")
	checklistPolicyFragment = fragmentTemplate("partials/checklist_policy_form.html")
)

// registerAdminChecklistPolicyRoutes wires the admin-only checklist-creation
// restriction settings page and its update fragment.
func registerAdminChecklistPolicyRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /admin/checklist-policy", requireAdminPage(handleAdminChecklistPolicyPage(store)))
	mux.Handle("PUT /admin/checklist-policy", api.RequireAuth(api.RequireAdmin(handleUpdateChecklistPolicyFragment(store))))
}

type checklistPolicyData struct {
	baseData
	Restrict bool
	Groups   []checklistPolicyGroupOption
	Saved    bool
	Error    string
}

// checklistPolicyGroupOption adds the currently-selected-creator-group flag
// to a domain.Group for the <select> — html/template's eq can't compare a
// *int64 against an int64 directly, so this is resolved in Go instead.
type checklistPolicyGroupOption struct {
	domain.Group
	Selected bool
}

func handleAdminChecklistPolicyPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		tenant, err := store.Tenants().GetByID(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		groups, err := store.Groups().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderPage(w, checklistPolicyTemplate, checklistPolicyDataFromTenant(tenant, groups, baseData{Actor: actor}, "", false))
	}
}

func checklistPolicyDataFromTenant(tenant *domain.Tenant, groups []domain.Group, base baseData, errMsg string, saved bool) checklistPolicyData {
	options := make([]checklistPolicyGroupOption, len(groups))
	for i, g := range groups {
		options[i] = checklistPolicyGroupOption{
			Group:    g,
			Selected: tenant.CreatorGroupID != nil && *tenant.CreatorGroupID == g.ID,
		}
	}
	return checklistPolicyData{
		baseData: base,
		Restrict: tenant.RestrictChecklistCreation,
		Groups:   options,
		Error:    errMsg,
		Saved:    saved,
	}
}

func handleUpdateChecklistPolicyFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())

		restrict := r.FormValue("restrict") != ""
		var groupID *int64
		if v := r.FormValue("creator_group_id"); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				renderChecklistPolicyResult(w, r, store, actor, "invalid creator group", false)
				return
			}
			groupID = &id
		}
		if restrict && groupID == nil {
			renderChecklistPolicyResult(w, r, store, actor, "a creator group is required when restriction is enabled", false)
			return
		}

		policy := domain.ChecklistCreationPolicy{Restrict: restrict, CreatorGroupID: groupID}
		if err := store.Tenants().UpdateChecklistCreationPolicy(r.Context(), actor.TenantID, policy); err != nil {
			renderChecklistPolicyResult(w, r, store, actor, "internal error", false)
			return
		}
		renderChecklistPolicyResult(w, r, store, actor, "", true)
	}
}

func renderChecklistPolicyResult(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User, errMsg string, saved bool) {
	tenant, err := store.Tenants().GetByID(r.Context(), actor.TenantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	groups, err := store.Groups().List(r.Context(), actor.TenantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderFragment(w, checklistPolicyFragment, "checklist_policy_form", checklistPolicyDataFromTenant(tenant, groups, baseData{}, errMsg, saved))
}
