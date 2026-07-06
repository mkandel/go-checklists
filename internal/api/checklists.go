package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// errItemNotFound is returned by handlers when a URL-supplied item id isn't
// present on the checklist's current item list.
var errItemNotFound = errors.New("api: item not found")

// registerChecklistRoutes wires the checklist lifecycle endpoints into mux,
// all gated behind RequireAuth.
func registerChecklistRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("POST /api/checklists", RequireAuth(handleCreateChecklist(store)))
	mux.Handle("GET /api/checklists", RequireAuth(handleListChecklists(store)))
	mux.Handle("GET /api/checklists/{id}", RequireAuth(handleGetChecklist(store)))
	mux.Handle("POST /api/checklists/{id}/claim", RequireAuth(handleClaimChecklist(store)))
	mux.Handle("POST /api/checklists/{id}/items/{itemID}/check", RequireAuth(handleCheckItem(store)))
	mux.Handle("POST /api/checklists/{id}/approve", RequireAuth(handleApproveChecklist(store)))
	mux.Handle("POST /api/checklists/{id}/reject", RequireAuth(handleRejectChecklist(store)))
	mux.Handle("POST /api/checklists/{id}/items", RequireAuth(handleAddItem(store)))
	mux.Handle("DELETE /api/checklists/{id}/items/{itemID}", RequireAuth(handleRemoveItem(store)))
	mux.Handle("PUT /api/checklists/{id}/items/order", RequireAuth(handleReorderItems(store)))
	mux.Handle("PUT /api/checklists/{id}/items/{itemID}/checked", RequireAuth(handleSetItemChecked(store)))
}

type createChecklistRequest struct {
	TemplateID      int64  `json:"template_id"`
	Name            string `json:"name"`
	AssignedGroupID *int64 `json:"assigned_group_id"`
	AssignedUserID  *int64 `json:"assigned_user_id"`
	Hidden          bool   `json:"hidden"`
	ApproverID      *int64 `json:"approver_id"`
}

func handleCreateChecklist(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createChecklistRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		tenant, err := store.Tenants().GetByID(r.Context(), actor.TenantID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		var isCreatorGroupMember bool
		if tenant.RestrictChecklistCreation && !actor.IsAdmin && tenant.CreatorGroupID != nil {
			var err error
			isCreatorGroupMember, err = store.Groups().IsMember(r.Context(), actor.TenantID, *tenant.CreatorGroupID, actor.ID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if err := domain.CanCreateChecklist(tenant, actor, isCreatorGroupMember); err != nil {
			writeDomainError(w, err)
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
			writeDomainError(w, err)
			return
		}

		c := &domain.Checklist{
			TenantID:        actor.TenantID,
			TemplateID:      req.TemplateID,
			Name:            req.Name,
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
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	}
}

func handleGetChecklist(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		c, err := store.Checklists().Get(r.Context(), actor.TenantID, id)
		if err != nil {
			writeDomainError(w, err)
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
		writeJSON(w, http.StatusOK, c)
	}
}

type claimRequest struct {
	ExpectedCurrentUserID *int64 `json:"expected_current_user_id"`
}

func handleClaimChecklist(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		var req claimRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		var claimed bool
		err := store.WithTx(r.Context(), func(tx *postgres.Store) error {
			var err error
			claimed, err = tx.Checklists().Claim(r.Context(), actor.TenantID, id, actor.ID, req.ExpectedCurrentUserID)
			return err
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if !claimed {
			http.Error(w, "checklist was claimed by someone else first", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleCheckItem(store *postgres.Store) http.HandlerFunc {
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
		actor, _ := UserFromContext(r.Context())

		withChecklistMutation(w, r, store, actor.TenantID, id, func(c *domain.Checklist) ([]domain.Event, error) {
			idx, ok := c.ItemIndex(itemID)
			if !ok {
				return nil, errItemNotFound
			}
			return c.CheckItem(idx, actor.ID, time.Now())
		})
	}
}

func handleApproveChecklist(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		withChecklistMutation(w, r, store, actor.TenantID, id, func(c *domain.Checklist) ([]domain.Event, error) {
			return c.Approve(actor.ID)
		})
	}
}

type rejectRequest struct {
	ItemIDs []int64 `json:"item_ids"`
}

func handleRejectChecklist(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		var req rejectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		withChecklistMutation(w, r, store, actor.TenantID, id, func(c *domain.Checklist) ([]domain.Event, error) {
			indices := make([]int, len(req.ItemIDs))
			for i, itemID := range req.ItemIDs {
				idx, ok := c.ItemIndex(itemID)
				if !ok {
					return nil, errItemNotFound
				}
				indices[i] = idx
			}
			return c.Reject(actor.ID, indices)
		})
	}
}

type addItemRequest struct {
	Name          string `json:"name"`
	ValidationRef string `json:"validation_ref"`
}

func handleAddItem(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		var req addItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		withChecklistMutation(w, r, store, actor.TenantID, id, func(c *domain.Checklist) ([]domain.Event, error) {
			return c.AddItem(actor.ID, req.Name, req.ValidationRef)
		})
	}
}

func handleRemoveItem(store *postgres.Store) http.HandlerFunc {
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
		actor, _ := UserFromContext(r.Context())

		withChecklistMutation(w, r, store, actor.TenantID, id, func(c *domain.Checklist) ([]domain.Event, error) {
			idx, ok := c.ItemIndex(itemID)
			if !ok {
				return nil, errItemNotFound
			}
			return c.RemoveItem(actor.ID, idx)
		})
	}
}

type reorderRequest struct {
	ItemIDs []int64 `json:"item_ids"`
}

func handleReorderItems(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid checklist id", http.StatusBadRequest)
			return
		}
		var req reorderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		withChecklistMutation(w, r, store, actor.TenantID, id, func(c *domain.Checklist) ([]domain.Event, error) {
			return c.ReorderItems(actor.ID, req.ItemIDs)
		})
	}
}

type setCheckedRequest struct {
	Checked bool `json:"checked"`
}

func handleSetItemChecked(store *postgres.Store) http.HandlerFunc {
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
		var req setCheckedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())

		withChecklistMutation(w, r, store, actor.TenantID, id, func(c *domain.Checklist) ([]domain.Event, error) {
			idx, ok := c.ItemIndex(itemID)
			if !ok {
				return nil, errItemNotFound
			}
			return c.SetItemChecked(actor.ID, idx, req.Checked, time.Now())
		})
	}
}

// withChecklistMutation runs the Get -> mutate -> Save sequence common to
// every checklist state-transition endpoint inside a single transaction
// (so the read that ChecklistRepo.Get locks FOR UPDATE stays held across
// the domain-method call and the Save it feeds), then writes the refreshed
// checklist as the response.
func withChecklistMutation(w http.ResponseWriter, r *http.Request, store *postgres.Store, tenantID, id int64, mutate func(*domain.Checklist) ([]domain.Event, error)) {
	var result *domain.Checklist
	err := store.WithTx(r.Context(), func(tx *postgres.Store) error {
		c, err := tx.Checklists().Get(r.Context(), tenantID, id)
		if err != nil {
			return err
		}
		events, err := mutate(c)
		if err != nil {
			return err
		}
		if err := tx.Checklists().Save(r.Context(), c, events); err != nil {
			return err
		}
		result = c
		return nil
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func pathInt64(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeDomainError(w http.ResponseWriter, err error) {
	status, msg := mapDomainError(err)
	http.Error(w, msg, status)
}

// mapDomainError translates domain/store errors from the checklist mutation
// pipeline into HTTP statuses.
func mapDomainError(err error) (int, string) {
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, errItemNotFound), errors.Is(err, postgres.ErrNotificationNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, domain.ErrChecklistNotOpen),
		errors.Is(err, domain.ErrNotValidating),
		errors.Is(err, domain.ErrUnclaimed):
		return http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrNotAssignee),
		errors.Is(err, domain.ErrNotApprover),
		errors.Is(err, domain.ErrNotCreator),
		errors.Is(err, domain.ErrChecklistCreationRestricted):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, domain.ErrInvalidReorder),
		errors.Is(err, domain.ErrAssignmentRequired),
		errors.Is(err, domain.ErrAssigneeNotGroupMember):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrUsernameTaken):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
