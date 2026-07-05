package web

import (
	"net/http"

	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	loginTemplate    = pageTemplate("login.html")
	registerTemplate = pageTemplate("register.html")
)

// registerAuthRoutes wires the login/register pages. The auth actions
// themselves (POST /login, POST /register, POST /logout) stay in
// internal/api unchanged — these are render-only GET pages for the forms
// that submit to those existing endpoints.
func registerAuthRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /login", handleLoginPage)
	mux.HandleFunc("GET /register", handleRegisterPage)
}

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, loginTemplate, baseData{})
}

func handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, registerTemplate, baseData{})
}
