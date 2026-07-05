package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	checklistsListTemplate = pageTemplate("checklists_list.html")
	checklistsNewTemplate  = pageTemplate("checklists_new.html")
)

// registerChecklistUIRoutes wires the checklist list and create pages. The
// detail page (check/claim/approve/reject/reorder) is a separate, later
// task — creating a checklist here redirects back to the list, not to a
// detail page, until that lands.
func registerChecklistUIRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /checklists", requireAuthPage(handleChecklistsListPage(store)))
	mux.HandleFunc("GET /checklists/new", requireAuthPage(handleChecklistsNewPage(store)))
	mux.Handle("POST /checklists", api.RequireAuth(handleCreateChecklistUI(store)))
}

// checklistRow adds display labels (resolved from ID to name) to a
// domain.Checklist for the list page, since html/template can't do map
// lookups against arbitrary IDs on its own.
type checklistRow struct {
	domain.Checklist
	AssigneeLabel string
	ApproverLabel string
	CreatorLabel  string
}

type checklistsListPageData struct {
	baseData
	Status    string
	Rows      []checklistRow
	CanCreate bool
}

func handleChecklistsListPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())

		filter := domain.ChecklistFilter{TenantID: actor.TenantID, UserID: actor.ID}
		statusParam := r.URL.Query().Get("status")
		if statusParam != "" {
			status := domain.ChecklistStatus(statusParam)
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
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		users, err := store.Users().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		groups, err := store.Groups().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		userNames := make(map[int64]string, len(users))
		for _, u := range users {
			userNames[u.ID] = u.Name
		}
		groupNames := make(map[int64]string, len(groups))
		for _, g := range groups {
			groupNames[g.ID] = g.Name
		}

		rows := make([]checklistRow, len(checklists))
		for i, c := range checklists {
			rows[i] = checklistRow{
				Checklist:     c,
				AssigneeLabel: assigneeLabel(c, groupNames, userNames),
				ApproverLabel: userLabelOrNone(c.ApproverID, userNames),
				CreatorLabel:  userNames[c.CreatorID],
			}
		}

		canCreate, err := canCreateChecklist(r.Context(), store, actor)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		renderPage(w, checklistsListTemplate, checklistsListPageData{
			baseData:  baseData{Actor: actor},
			Status:    statusParam,
			Rows:      rows,
			CanCreate: canCreate,
		})
	}
}

func assigneeLabel(c domain.Checklist, groupNames, userNames map[int64]string) string {
	switch {
	case c.AssignedGroupID != nil && c.AssignedUserID != nil:
		return groupNames[*c.AssignedGroupID] + " / " + userNames[*c.AssignedUserID]
	case c.AssignedGroupID != nil:
		return groupNames[*c.AssignedGroupID]
	case c.AssignedUserID != nil:
		return userNames[*c.AssignedUserID]
	default:
		return "Unassigned"
	}
}

func userLabelOrNone(id *int64, userNames map[int64]string) string {
	if id == nil {
		return "None"
	}
	return userNames[*id]
}

// canCreateChecklist reports whether actor may create a checklist for their
// tenant, per the tenant's (optional) checklist-creation restriction — see
// domain.CanCreateChecklist. The bool return is a business-rule verdict; the
// error return is an infrastructure failure (DB error) the caller should
// treat as an internal error.
func canCreateChecklist(ctx context.Context, store *postgres.Store, actor *domain.User) (bool, error) {
	tenant, err := store.Tenants().GetByID(ctx, actor.TenantID)
	if err != nil {
		return false, err
	}
	var isCreatorGroupMember bool
	if tenant.RestrictChecklistCreation && !actor.IsAdmin && tenant.CreatorGroupID != nil {
		isCreatorGroupMember, err = store.Groups().IsMember(ctx, actor.TenantID, *tenant.CreatorGroupID, actor.ID)
		if err != nil {
			return false, err
		}
	}
	return domain.CanCreateChecklist(tenant, actor, isCreatorGroupMember) == nil, nil
}

type checklistsNewPageData struct {
	baseData
	Groups         []domain.Group
	Users          []domain.User
	TemplateGroups []templateGroup
}

func handleChecklistsNewPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())

		canCreate, err := canCreateChecklist(r.Context(), store, actor)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !canCreate {
			http.Error(w, domain.ErrChecklistCreationRestricted.Error(), http.StatusForbidden)
			return
		}

		groups, err := store.Groups().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		users, err := store.Users().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		templates, err := store.Templates().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		renderPage(w, checklistsNewTemplate, checklistsNewPageData{
			baseData:       baseData{Actor: actor},
			Groups:         groups,
			Users:          users,
			TemplateGroups: groupTemplatesByName(templates),
		})
	}
}

type createChecklistUIRequest struct {
	TemplateID      int64  `json:"template_id"`
	AssignedGroupID *int64 `json:"assigned_group_id"`
	AssignedUserID  *int64 `json:"assigned_user_id"`
	Hidden          bool   `json:"hidden"`
	ApproverID      *int64 `json:"approver_id"`
}

// handleCreateChecklistUI mirrors internal/api's handleCreateChecklist
// (small, easy-to-duplicate amount of logic, per this project's established
// preference for keeping internal/web decoupled from internal/api's
// internals) — called via fetch() from checklists_new.html.
func handleCreateChecklistUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createChecklistUIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())

		canCreate, err := canCreateChecklist(r.Context(), store, actor)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !canCreate {
			http.Error(w, domain.ErrChecklistCreationRestricted.Error(), http.StatusForbidden)
			return
		}

		var isMember bool
		if req.AssignedGroupID != nil && req.AssignedUserID != nil {
			var err error
			isMember, err = store.Groups().IsMember(r.Context(), actor.TenantID, *req.AssignedGroupID, *req.AssignedUserID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if err := domain.ValidateAssignment(req.AssignedGroupID, req.AssignedUserID, isMember); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		c := &domain.Checklist{
			TenantID:        actor.TenantID,
			TemplateID:      req.TemplateID,
			CreatorID:       actor.ID,
			AssignedGroupID: req.AssignedGroupID,
			AssignedUserID:  req.AssignedUserID,
			Hidden:          req.Hidden,
			ApproverID:      req.ApproverID,
		}

		err = store.WithTx(r.Context(), func(tx *postgres.Store) error {
			return tx.Checklists().Create(r.Context(), c)
		})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(struct {
			ID int64 `json:"id"`
		}{ID: c.ID})
	}
}
