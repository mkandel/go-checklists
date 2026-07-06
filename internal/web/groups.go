package web

import (
	"net/http"
	"strconv"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	groupsListTemplate   = pageTemplate("groups_list.html", "partials/groups_table.html")
	groupsTableFragment  = fragmentTemplate("partials/groups_table.html")
	groupMembersFragment = fragmentTemplate("partials/group_members.html")
)

// registerGroupRoutes wires the groups page and its htmx fragments. Reads
// are available to any signed-in user (mirroring internal/api's own
// registerGroupRoutes); mutations are admin-only.
func registerGroupRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /groups", requireAuthPage(handleGroupsPage(store)))
	mux.Handle("GET /groups/{id}/members", api.RequireAuth(handleGroupMembersFragment(store)))
	mux.Handle("POST /groups", api.RequireAuth(api.RequireAdmin(handleCreateGroupFragment(store))))
	mux.Handle("POST /groups/{id}/members", api.RequireAuth(api.RequireAdmin(handleAddGroupMemberFragment(store))))
	mux.Handle("DELETE /groups/{id}/members/{userID}", api.RequireAuth(api.RequireAdmin(handleRemoveGroupMemberFragment(store))))
}

type groupsPageData struct {
	baseData
	IsAdmin bool
	Groups  []domain.Group
}

func handleGroupsPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		groups, err := store.Groups().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderPage(w, groupsListTemplate, groupsPageData{
			baseData: baseData{Actor: actor},
			IsAdmin:  actor.IsAdmin,
			Groups:   groups,
		})
	}
}

func handleCreateGroupFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		g := &domain.Group{TenantID: actor.TenantID, Name: r.FormValue("name")}
		if err := store.Groups().Create(r.Context(), g); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderGroupsTable(w, r, store, actor)
	}
}

func renderGroupsTable(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User) {
	groups, err := store.Groups().List(r.Context(), actor.TenantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderFragment(w, groupsTableFragment, "groups_table", groupsPageData{
		IsAdmin: actor.IsAdmin,
		Groups:  groups,
	})
}

type groupMembersData struct {
	IsAdmin        bool
	GroupID        int64
	Members        []domain.User
	AvailableUsers []domain.User
	ShowInactive   bool
}

func handleGroupMembersFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid group id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		showInactive := r.URL.Query().Get("show_inactive") == "1"
		renderGroupMembers(w, r, store, actor, id, showInactive)
	}
}

func renderGroupMembers(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User, groupID int64, showInactive bool) {
	members, err := store.Groups().ListMembers(r.Context(), actor.TenantID, groupID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := groupMembersData{IsAdmin: actor.IsAdmin, GroupID: groupID, Members: members, ShowInactive: showInactive}
	if actor.IsAdmin {
		allUsers, err := store.Users().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		isMember := make(map[int64]bool, len(members))
		for _, m := range members {
			isMember[m.ID] = true
		}
		for _, u := range allUsers {
			if isMember[u.ID] {
				continue
			}
			if !u.IsActive && !showInactive {
				continue
			}
			data.AvailableUsers = append(data.AvailableUsers, u)
		}
	}
	renderFragment(w, groupMembersFragment, "group_members", data)
}

func handleAddGroupMemberFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid group id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		userID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		showInactive := r.FormValue("show_inactive") == "1"
		actor, _ := api.UserFromContext(r.Context())
		if err := store.Groups().AddMember(r.Context(), actor.TenantID, id, userID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderGroupMembers(w, r, store, actor, id, showInactive)
	}
}

func handleRemoveGroupMemberFragment(store *postgres.Store) http.HandlerFunc {
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
		showInactive := r.URL.Query().Get("show_inactive") == "1"
		actor, _ := api.UserFromContext(r.Context())
		if err := store.Groups().RemoveMember(r.Context(), actor.TenantID, id, userID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderGroupMembers(w, r, store, actor, id, showInactive)
	}
}
