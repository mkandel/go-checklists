package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	checklistDetailTemplate = pageTemplate("checklist_detail.html", "partials/checklist_panel.html")
	checklistPanelFragment  = fragmentTemplate("partials/checklist_panel.html")
)

// errItemNotFoundUI mirrors internal/api's identical unexported sentinel,
// for a URL-supplied item id that isn't on this checklist.
var errItemNotFoundUI = errors.New("web: item not found")

// registerChecklistDetailRoutes wires the checklist detail page and its
// state-machine-gated mutation fragments. Every mutation endpoint renders
// and returns the same "checklist_panel" fragment (with an inline
// FlashError on domain-rule violations, rather than a raw HTTP error body),
// so a single htmx swap target covers every action on the page.
func registerChecklistDetailRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /checklists/{id}", requireAuthPage(handleChecklistDetailPage(store)))
	mux.Handle("POST /checklists/{id}/claim", api.RequireAuth(handleClaimChecklistUI(store)))
	mux.Handle("POST /checklists/{id}/items/{itemID}/check", api.RequireAuth(handleCheckItemUI(store)))
	mux.Handle("PUT /checklists/{id}/items/{itemID}/checked", api.RequireAuth(handleSetItemCheckedUI(store)))
	mux.Handle("POST /checklists/{id}/approve", api.RequireAuth(handleApproveChecklistUI(store)))
	mux.Handle("POST /checklists/{id}/reject", api.RequireAuth(handleRejectChecklistUI(store)))
	mux.Handle("POST /checklists/{id}/items", api.RequireAuth(handleAddItemUI(store)))
	mux.Handle("DELETE /checklists/{id}/items/{itemID}", api.RequireAuth(handleRemoveItemUI(store)))
	mux.Handle("PUT /checklists/{id}/items/order", api.RequireAuth(handleReorderItemsUI(store)))
}

// checklistItemView adds display labels to a domain.ChecklistItem for the
// detail page, since html/template can't do map lookups or call
// Checklist.ResponsibleUserFor against arbitrary IDs on its own.
type checklistItemView struct {
	domain.ChecklistItem
	ResponsibleLabel string
	CheckedByLabel   string
	CanCheck         bool
}

// checklistPanelData is the data behind the "checklist_panel" fragment —
// everything on the page that changes as a result of a mutation. It's
// embedded (unexported, like baseData) in checklistDetailPageData for the
// full-page render, and passed directly for every fragment re-render after
// a mutation, mirroring the notificationsPageData/notifications_table
// pattern.
type checklistPanelData struct {
	Checklist            domain.Checklist
	Items                []checklistItemView
	AssigneeLabel        string
	ApproverLabel        string
	CreatorLabel         string
	IsCreator            bool
	IsApproverValidating bool
	CanClaim             bool
	FlashError           string
}

type checklistDetailPageData struct {
	baseData
	checklistPanelData
}

func handleChecklistDetailPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())

		c, err := store.Checklists().Get(r.Context(), actor.TenantID, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var isMember bool
		if c.Hidden && c.AssignedUserID == nil && c.AssignedGroupID != nil {
			isMember, err = store.Groups().IsMember(r.Context(), actor.TenantID, *c.AssignedGroupID, actor.ID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if !c.VisibleTo(actor.ID, isMember) {
			http.NotFound(w, r)
			return
		}

		panel, err := buildChecklistPanelData(r, store, actor, c, "")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderPage(w, checklistDetailTemplate, checklistDetailPageData{
			baseData:           baseData{Actor: actor},
			checklistPanelData: panel,
		})
	}
}

func buildChecklistPanelData(r *http.Request, store *postgres.Store, actor *domain.User, c *domain.Checklist, flashErr string) (checklistPanelData, error) {
	users, err := store.Users().List(r.Context(), actor.TenantID)
	if err != nil {
		return checklistPanelData{}, err
	}
	groups, err := store.Groups().List(r.Context(), actor.TenantID)
	if err != nil {
		return checklistPanelData{}, err
	}
	userNames := make(map[int64]string, len(users))
	for _, u := range users {
		userNames[u.ID] = u.Name
	}
	groupNames := make(map[int64]string, len(groups))
	for _, g := range groups {
		groupNames[g.ID] = g.Name
	}

	items := make([]checklistItemView, len(c.Items))
	for i, item := range c.Items {
		view := checklistItemView{ChecklistItem: item}
		if responsible := c.ResponsibleUserFor(item); responsible != nil {
			view.ResponsibleLabel = userNames[*responsible]
			view.CanCheck = c.Status == domain.StatusOpen && !item.Checked && *responsible == actor.ID
		}
		if item.CheckedBy != nil {
			view.CheckedByLabel = userNames[*item.CheckedBy]
		}
		items[i] = view
	}

	return checklistPanelData{
		Checklist:            *c,
		Items:                items,
		AssigneeLabel:        assigneeLabel(*c, groupNames, userNames),
		ApproverLabel:        userLabelOrNone(c.ApproverID, userNames),
		CreatorLabel:         userNames[c.CreatorID],
		IsCreator:            actor.ID == c.CreatorID,
		IsApproverValidating: c.Status == domain.StatusValidating && c.ApproverID != nil && *c.ApproverID == actor.ID,
		CanClaim:             c.AssignedUserID == nil,
		FlashError:           flashErr,
	}, nil
}

func renderChecklistPanel(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User, id int64, flashErr string) {
	c, err := store.Checklists().Get(r.Context(), actor.TenantID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	panel, err := buildChecklistPanelData(r, store, actor, c, flashErr)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderFragment(w, checklistPanelFragment, "checklist_panel", panel)
}

// withChecklistMutationUI mirrors internal/api's withChecklistMutation
// (Get -> mutate -> Save in one transaction), but on a domain-rule error
// re-renders the panel fragment with FlashError set (still HTTP 200) instead
// of returning a raw error body — htmx doesn't swap non-2xx responses by
// default, and an inline banner is friendlier for a page a user is actively
// working in. A checklist/item that doesn't exist at all still 404s.
func withChecklistMutationUI(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User, id int64, mutate func(*domain.Checklist) ([]domain.Event, error)) {
	err := store.WithTx(r.Context(), func(tx *postgres.Store) error {
		c, err := tx.Checklists().Get(r.Context(), actor.TenantID, id)
		if err != nil {
			return err
		}
		events, err := mutate(c)
		if err != nil {
			return err
		}
		return tx.Checklists().Save(r.Context(), c, events)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, errItemNotFoundUI) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		renderChecklistPanel(w, r, store, actor, id, err.Error())
		return
	}
	renderChecklistPanel(w, r, store, actor, id, "")
}

func handleClaimChecklistUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())

		c, err := store.Checklists().Get(r.Context(), actor.TenantID, id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		claimed, err := store.Checklists().Claim(r.Context(), actor.TenantID, id, actor.ID, c.AssignedUserID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !claimed {
			renderChecklistPanel(w, r, store, actor, id, "someone else claimed this checklist first")
			return
		}
		renderChecklistPanel(w, r, store, actor, id, "")
	}
}

func handleCheckItemUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		itemID, ok := pathInt64(r, "itemID")
		if !ok {
			http.Error(w, "invalid item id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		withChecklistMutationUI(w, r, store, actor, id, func(c *domain.Checklist) ([]domain.Event, error) {
			idx, ok := c.ItemIndex(itemID)
			if !ok {
				return nil, errItemNotFoundUI
			}
			return c.CheckItem(idx, actor.ID, time.Now())
		})
	}
}

func handleSetItemCheckedUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		itemID, ok := pathInt64(r, "itemID")
		if !ok {
			http.Error(w, "invalid item id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		checked := r.FormValue("checked") == "true"
		actor, _ := api.UserFromContext(r.Context())
		withChecklistMutationUI(w, r, store, actor, id, func(c *domain.Checklist) ([]domain.Event, error) {
			idx, ok := c.ItemIndex(itemID)
			if !ok {
				return nil, errItemNotFoundUI
			}
			return c.SetItemChecked(actor.ID, idx, checked, time.Now())
		})
	}
}

func handleApproveChecklistUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		withChecklistMutationUI(w, r, store, actor, id, func(c *domain.Checklist) ([]domain.Event, error) {
			return c.Approve(actor.ID)
		})
	}
}

func handleRejectChecklistUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())

		itemIDs := make([]int64, 0, len(r.Form["item_id"]))
		for _, raw := range r.Form["item_id"] {
			itemID, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				http.Error(w, "invalid item id", http.StatusBadRequest)
				return
			}
			itemIDs = append(itemIDs, itemID)
		}

		withChecklistMutationUI(w, r, store, actor, id, func(c *domain.Checklist) ([]domain.Event, error) {
			indices := make([]int, len(itemIDs))
			for i, itemID := range itemIDs {
				idx, ok := c.ItemIndex(itemID)
				if !ok {
					return nil, errItemNotFoundUI
				}
				indices[i] = idx
			}
			return c.Reject(actor.ID, indices)
		})
	}
}

func handleAddItemUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		validationRef := r.FormValue("validation_ref")
		actor, _ := api.UserFromContext(r.Context())
		withChecklistMutationUI(w, r, store, actor, id, func(c *domain.Checklist) ([]domain.Event, error) {
			return c.AddItem(actor.ID, name, validationRef)
		})
	}
}

func handleRemoveItemUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		itemID, ok := pathInt64(r, "itemID")
		if !ok {
			http.Error(w, "invalid item id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		withChecklistMutationUI(w, r, store, actor, id, func(c *domain.Checklist) ([]domain.Event, error) {
			idx, ok := c.ItemIndex(itemID)
			if !ok {
				return nil, errItemNotFoundUI
			}
			return c.RemoveItem(actor.ID, idx)
		})
	}
}

type reorderUIRequest struct {
	ItemIDs []int64 `json:"item_ids"`
}

// handleReorderItemsUI is the one mutation endpoint on this page driven by
// JS (checklist_detail.html's SortableJS onEnd handler) rather than a plain
// htmx form, since the new order comes from the dragged DOM state rather
// than form fields. Like every other endpoint here it renders and returns
// the checklist_panel fragment as its response body.
func handleReorderItemsUI(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		var req reorderUIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		withChecklistMutationUI(w, r, store, actor, id, func(c *domain.Checklist) ([]domain.Event, error) {
			return c.ReorderItems(actor.ID, req.ItemIDs)
		})
	}
}
