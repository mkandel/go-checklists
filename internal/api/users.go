package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// registerUserRoutes wires the user directory and admin user-management
// endpoints into mux. Listing is available to any signed-in user; creating
// users (single or bulk) is admin-only.
func registerUserRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /users", RequireAuth(handleListUsers(store)))
	mux.Handle("POST /admin/users", RequireAuth(RequireAdmin(handleAdminCreateUser(store))))
	mux.Handle("POST /admin/users/bulk", RequireAuth(RequireAdmin(handleAdminBulkCreateUsers(store))))
}

func handleListUsers(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := UserFromContext(r.Context())
		users, err := store.Users().List(r.Context(), actor.TenantID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	IsAdmin  bool   `json:"is_admin"`
}

// handleAdminCreateUser lets an admin provision a single user directly into
// their own tenant, active immediately (unlike self-service registration,
// there's no need to log the new user in here — the admin stays logged in
// as themselves).
func handleAdminCreateUser(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Name == "" || req.Password == "" {
			http.Error(w, "username, password, and name are required", http.StatusBadRequest)
			return
		}

		actor, _ := UserFromContext(r.Context())
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		u := &domain.User{
			TenantID:     actor.TenantID,
			Name:         req.Name,
			Username:     req.Username,
			PasswordHash: hash,
			IsAdmin:      req.IsAdmin,
			IsActive:     true,
		}
		if err := store.Users().Create(r.Context(), u); err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, u)
	}
}

// bulkCreateUserResult reports the outcome of one row of a bulk CSV
// user-creation request.
type bulkCreateUserResult struct {
	Row      int          `json:"row"`
	Username string       `json:"username,omitempty"`
	Status   string       `json:"status"`
	Error    string       `json:"error,omitempty"`
	User     *domain.User `json:"user,omitempty"`
}

// handleAdminBulkCreateUsers creates multiple users from a CSV request
// body — one row per user, no header, columns `username,password,name` with
// an optional 4th `is_admin` column (blank or omitted defaults to false).
// Rows are processed independently: a bad row (duplicate username, missing
// field) doesn't abort the batch — the response is a per-row result list so
// the caller can see exactly which rows succeeded.
func handleAdminBulkCreateUsers(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := UserFromContext(r.Context())

		reader := csv.NewReader(r.Body)
		reader.FieldsPerRecord = -1
		reader.TrimLeadingSpace = true

		var results []bulkCreateUserResult
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

			username, password, name, isAdmin, err := parseBulkUserRow(record)
			if err != nil {
				results = append(results, bulkCreateUserResult{Row: row, Status: "error", Error: err.Error()})
				continue
			}

			hash, err := auth.HashPassword(password)
			if err != nil {
				results = append(results, bulkCreateUserResult{Row: row, Username: username, Status: "error", Error: "internal error"})
				continue
			}
			u := &domain.User{
				TenantID:     actor.TenantID,
				Name:         name,
				Username:     username,
				PasswordHash: hash,
				IsAdmin:      isAdmin,
				IsActive:     true,
			}
			if err := store.Users().Create(r.Context(), u); err != nil {
				msg := "internal error"
				if errors.Is(err, domain.ErrUsernameTaken) {
					msg = err.Error()
				}
				results = append(results, bulkCreateUserResult{Row: row, Username: username, Status: "error", Error: msg})
				continue
			}
			results = append(results, bulkCreateUserResult{Row: row, Username: username, Status: "created", User: u})
		}

		writeJSON(w, http.StatusOK, results)
	}
}

// parseBulkUserRow validates and extracts one CSV record for
// handleAdminBulkCreateUsers.
func parseBulkUserRow(record []string) (username, password, name string, isAdmin bool, err error) {
	if len(record) < 3 {
		return "", "", "", false, errors.New("expected at least 3 columns: username,password,name[,is_admin]")
	}
	username, password, name = record[0], record[1], record[2]
	if username == "" || password == "" || name == "" {
		return "", "", "", false, errors.New("username, password, and name are required")
	}
	if len(record) >= 4 && record[3] != "" {
		isAdmin, err = strconv.ParseBool(record[3])
		if err != nil {
			return "", "", "", false, fmt.Errorf("invalid is_admin value %q", record[3])
		}
	}
	return username, password, name, isAdmin, nil
}
