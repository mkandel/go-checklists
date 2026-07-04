package api

import (
	"encoding/json"
	"net/http"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// registerGroupRoutes wires the group endpoints into mux. Reads are
// available to any signed-in user (to pick a group when creating a
// checklist); mutations are admin-only.
func registerGroupRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /groups", RequireAuth(handleListGroups(store)))
	mux.Handle("GET /groups/{id}/members", RequireAuth(handleListGroupMembers(store)))
	mux.Handle("POST /groups", RequireAuth(RequireAdmin(handleCreateGroup(store))))
	mux.Handle("POST /groups/{id}/members", RequireAuth(RequireAdmin(handleAddGroupMember(store))))
	mux.Handle("DELETE /groups/{id}/members/{userID}", RequireAuth(RequireAdmin(handleRemoveGroupMember(store))))
}

func handleListGroups(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groups, err := store.Groups().List(r.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, groups)
	}
}

func handleListGroupMembers(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid group id", http.StatusBadRequest)
			return
		}
		members, err := store.Groups().ListMembers(r.Context(), id)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, members)
	}
}

type createGroupRequest struct {
	Name string `json:"name"`
}

func handleCreateGroup(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createGroupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		g := &domain.Group{Name: req.Name}
		if err := store.Groups().Create(r.Context(), g); err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, g)
	}
}

type addGroupMemberRequest struct {
	UserID int64 `json:"user_id"`
}

func handleAddGroupMember(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid group id", http.StatusBadRequest)
			return
		}
		var req addGroupMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := store.Groups().AddMember(r.Context(), id, req.UserID); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRemoveGroupMember(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid group id", http.StatusBadRequest)
			return
		}
		userID, ok := pathInt64(r, "userID")
		if !ok {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		if err := store.Groups().RemoveMember(r.Context(), id, userID); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
