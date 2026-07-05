package web

import (
	"net/http"

	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	loginTemplate                = pageTemplate("login.html")
	registerTemplate             = pageTemplate("register.html")
	passwordResetRequestTemplate = pageTemplate("password_reset_request.html")
	passwordResetConfirmTemplate = pageTemplate("password_reset_confirm.html")
)

// registerAuthRoutes wires the login/register/password-reset pages. The auth
// actions themselves (POST /login, POST /register, POST /logout,
// POST /password-reset/request, POST /password-reset/confirm) stay in
// internal/api unchanged — these are render-only GET pages for the forms
// that submit to those existing endpoints.
func registerAuthRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /login", handleLoginPage)
	mux.HandleFunc("GET /register", handleRegisterPage)
	mux.HandleFunc("GET /password-reset", handlePasswordResetRequestPage)
	mux.HandleFunc("GET /password-reset/confirm", handlePasswordResetConfirmPage)
}

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, loginTemplate, baseData{})
}

func handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, registerTemplate, baseData{})
}

func handlePasswordResetRequestPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, passwordResetRequestTemplate, baseData{})
}

// passwordResetConfirmPageData carries the token from the query string
// (?token=...) into the confirm form as a hidden field, so the page doesn't
// need any client-side JS to read the URL itself.
type passwordResetConfirmPageData struct {
	baseData
	Token string
}

func handlePasswordResetConfirmPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, passwordResetConfirmTemplate, passwordResetConfirmPageData{
		Token: r.URL.Query().Get("token"),
	})
}
