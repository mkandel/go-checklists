package web

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	adminUsersTemplate = pageTemplate("admin_users.html", "partials/users_table.html")
	usersTableFragment = fragmentTemplate("partials/users_table.html")
	bulkResultFragment = fragmentTemplate("partials/bulk_upload_result.html")
)

// registerAdminUserRoutes wires the admin users page and its fragments.
// Every route here is admin-only — there's no non-admin view of this page.
func registerAdminUserRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /admin/users", requireAdminPage(handleAdminUsersPage(store)))
	mux.Handle("GET /admin/users/table", api.RequireAuth(api.RequireAdmin(handleUsersTableFragment(store))))
	mux.Handle("POST /admin/users", api.RequireAuth(api.RequireAdmin(handleCreateUserFragment(store))))
	mux.Handle("POST /admin/users/bulk", api.RequireAuth(api.RequireAdmin(handleBulkCreateUsersFragment(store))))
	mux.Handle("POST /admin/users/{id}/active", api.RequireAuth(api.RequireAdmin(handleSetUserActiveFragment(store))))
	mux.Handle("GET /admin/users/export.csv", api.RequireAuth(api.RequireAdmin(handleExportUsersCSV(store))))
}

// usersListSortColumns allowlists the ?sort= values accepted from the admin
// users list — kept in sync with userSortColumns in
// internal/store/postgres/users.go.
var usersListSortColumns = map[string]bool{"name": true, "username": true, "email": true, "is_admin": true, "is_active": true}

type usersPageData struct {
	baseData
	Users        []domain.User
	Sort         string
	Dir          string
	ShowInactive bool
}

func handleAdminUsersPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		filter, showInactive := usersFilterFromRequest(r, actor.TenantID)
		users, err := store.Users().ListFiltered(r.Context(), filter)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderPage(w, adminUsersTemplate, usersPageData{
			baseData:     baseData{Actor: actor},
			Users:        users,
			Sort:         filter.SortBy,
			Dir:          filter.SortDir,
			ShowInactive: showInactive,
		})
	}
}

func handleUsersTableFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		renderUsersTable(w, r, store, actor)
	}
}

// usersFilterFromRequest parses the ?sort=/?dir=/?show_inactive= query
// params shared by the admin users page and its htmx table fragment.
func usersFilterFromRequest(r *http.Request, tenantID int64) (filter domain.UserFilter, showInactive bool) {
	filter.TenantID = tenantID
	sortParam := r.URL.Query().Get("sort")
	if usersListSortColumns[sortParam] {
		filter.SortBy = sortParam
	}
	if r.URL.Query().Get("dir") == "desc" {
		filter.SortDir = "desc"
	}
	showInactive = r.URL.Query().Get("show_inactive") == "1"
	filter.IncludeInactive = showInactive
	return filter, showInactive
}

func renderUsersTable(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User) {
	filter, showInactive := usersFilterFromRequest(r, actor.TenantID)
	users, err := store.Users().ListFiltered(r.Context(), filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderFragment(w, usersTableFragment, "users_table", usersPageData{
		Users:        users,
		Sort:         filter.SortBy,
		Dir:          filter.SortDir,
		ShowInactive: showInactive,
	})
}

// emailPtr mirrors internal/api's identical helper: an empty string means no
// email, otherwise a pointer to it.
func emailPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func handleCreateUserFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		username := r.FormValue("username")
		name := r.FormValue("name")
		password := r.FormValue("password")
		if username == "" || name == "" || password == "" {
			http.Error(w, "username, password, and name are required", http.StatusBadRequest)
			return
		}

		actor, _ := api.UserFromContext(r.Context())
		hash, err := auth.HashPassword(password)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		u := &domain.User{
			TenantID:     actor.TenantID,
			Name:         name,
			Username:     username,
			PasswordHash: hash,
			Email:        emailPtr(r.FormValue("email")),
			IsAdmin:      r.FormValue("is_admin") == "true",
			IsActive:     true,
		}
		if err := store.Users().Create(r.Context(), u); err != nil {
			status := http.StatusInternalServerError
			msg := "internal error"
			if errors.Is(err, domain.ErrUsernameTaken) {
				status, msg = http.StatusConflict, err.Error()
			}
			http.Error(w, msg, status)
			return
		}
		renderUsersTable(w, r, store, actor)
	}
}

// bulkUserResult reports the outcome of one row of a bulk CSV
// user-creation request, mirroring internal/api's bulkCreateUserResult
// shape but rendered as HTML instead of JSON.
type bulkUserResult struct {
	Row      int
	Username string
	Status   string
	Error    string
}

// handleBulkCreateUsersFragment creates multiple users from a raw CSV
// request body — see internal/api's handleAdminBulkCreateUsers for the
// identical column format and per-row-independent-failure behavior this
// duplicates for the HTML-rendering front-end.
func handleBulkCreateUsersFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())

		reader := csv.NewReader(r.Body)
		reader.FieldsPerRecord = -1
		reader.TrimLeadingSpace = true

		var results []bulkUserResult
		row := 0
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "malformed CSV: "+err.Error(), http.StatusBadRequest)
				return
			}
			row++

			username, password, name, isAdmin, email, err := parseBulkUserRow(record)
			if err != nil {
				results = append(results, bulkUserResult{Row: row, Status: "error", Error: err.Error()})
				continue
			}

			hash, err := auth.HashPassword(password)
			if err != nil {
				results = append(results, bulkUserResult{Row: row, Username: username, Status: "error", Error: "internal error"})
				continue
			}
			u := &domain.User{
				TenantID:     actor.TenantID,
				Name:         name,
				Username:     username,
				PasswordHash: hash,
				Email:        emailPtr(email),
				IsAdmin:      isAdmin,
				IsActive:     true,
			}
			if err := store.Users().Create(r.Context(), u); err != nil {
				msg := "internal error"
				if errors.Is(err, domain.ErrUsernameTaken) {
					msg = err.Error()
				}
				results = append(results, bulkUserResult{Row: row, Username: username, Status: "error", Error: msg})
				continue
			}
			results = append(results, bulkUserResult{Row: row, Username: username, Status: "created"})
		}

		renderFragment(w, bulkResultFragment, "bulk_upload_result", struct{ Results []bulkUserResult }{Results: results})
	}
}

// handleExportUsersCSV writes every user in the tenant as CSV, in the same
// column order the bulk-upload form accepts (minus password, which isn't
// recoverable from the stored hash) so the file can be edited and used as a
// starting point for bulk changes.
func handleExportUsersCSV(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		users, err := store.Users().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="users.csv"`)

		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"username", "name", "email", "is_admin", "is_active"})
		for _, u := range users {
			email := ""
			if u.Email != nil {
				email = *u.Email
			}
			_ = writer.Write([]string{
				u.Username,
				u.Name,
				email,
				strconv.FormatBool(u.IsAdmin),
				strconv.FormatBool(u.IsActive),
			})
		}
		writer.Flush()
	}
}

// handleSetUserActiveFragment suspends or reactivates a user (there's no
// hard delete — see domain.UserRepo.SetActive). Refuses to let an admin
// suspend their own account, since that would lock them out with no other
// admin necessarily available to undo it. Suspending also clears the user
// as approver/assignee from any checklist that still points at them (see
// domain.ChecklistRepo.ClearUserAssignments) — they can no longer act on
// those checklists once suspended, so leaving the stale pointer in place
// would just make the checklist look actionable when it isn't.
func handleSetUserActiveFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		active, err := strconv.ParseBool(r.FormValue("active"))
		if err != nil {
			http.Error(w, "invalid active value", http.StatusBadRequest)
			return
		}

		actor, _ := api.UserFromContext(r.Context())
		if !active && id == actor.ID {
			http.Error(w, "you can't suspend your own account", http.StatusForbidden)
			return
		}

		err = store.WithTx(r.Context(), func(tx *postgres.Store) error {
			if err := tx.Users().SetActive(r.Context(), actor.TenantID, id, active); err != nil {
				return err
			}
			if !active {
				return tx.Checklists().ClearUserAssignments(r.Context(), actor.TenantID, id)
			}
			return nil
		})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderUsersTable(w, r, store, actor)
	}
}

// parseBulkUserRow mirrors internal/api's identical helper.
func parseBulkUserRow(record []string) (username, password, name string, isAdmin bool, email string, err error) {
	if len(record) < 3 {
		return "", "", "", false, "", errors.New("expected at least 3 columns: username,password,name[,is_admin[,email]]")
	}
	username, password, name = record[0], record[1], record[2]
	if username == "" || password == "" || name == "" {
		return "", "", "", false, "", errors.New("username, password, and name are required")
	}
	if len(record) >= 4 && record[3] != "" {
		isAdmin, err = strconv.ParseBool(record[3])
		if err != nil {
			return "", "", "", false, "", fmt.Errorf("invalid is_admin value %q", record[3])
		}
	}
	if len(record) >= 5 {
		email = record[4]
	}
	return username, password, name, isAdmin, email, nil
}
