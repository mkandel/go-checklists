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
}

type usersPageData struct {
	baseData
	Users []domain.User
}

func handleAdminUsersPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		users, err := store.Users().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderPage(w, adminUsersTemplate, usersPageData{
			baseData: baseData{Actor: actor},
			Users:    users,
		})
	}
}

func handleUsersTableFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		renderUsersTable(w, r, store, actor)
	}
}

func renderUsersTable(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User) {
	users, err := store.Users().List(r.Context(), actor.TenantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderFragment(w, usersTableFragment, "users_table", usersPageData{Users: users})
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
