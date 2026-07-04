package api

import (
	"net/http"

	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// registerUserRoutes wires the user directory endpoint into mux, gated
// behind RequireAuth.
func registerUserRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /users", RequireAuth(handleListUsers(store)))
}

func handleListUsers(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := store.Users().List(r.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}
