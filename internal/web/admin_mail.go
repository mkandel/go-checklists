package web

import (
	"net/http"
	"strconv"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	mailConfigTemplate = pageTemplate("admin_mail_config.html", "partials/mail_config_form.html")
	mailConfigFragment = fragmentTemplate("partials/mail_config_form.html")
)

// registerAdminMailRoutes wires the admin-only tenant SMTP config page and
// its update fragment.
func registerAdminMailRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /admin/mail-config", requireAdminPage(handleAdminMailConfigPage(store)))
	mux.Handle("PUT /admin/mail-config", api.RequireAuth(api.RequireAdmin(handleUpdateMailConfigFragment(store))))
}

type mailConfigData struct {
	baseData
	Host, Username, FromAddress string
	Port                        int
	Configured                  bool
	Saved                       bool
	Error                       string
}

func handleAdminMailConfigPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		tenant, err := store.Tenants().GetByID(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderPage(w, mailConfigTemplate, mailConfigDataFromTenant(tenant, baseData{Actor: actor}, "", false))
	}
}

func mailConfigDataFromTenant(tenant *domain.Tenant, base baseData, errMsg string, saved bool) mailConfigData {
	return mailConfigData{
		baseData:    base,
		Host:        strOr(tenant.SMTPHost),
		Port:        intOr(tenant.SMTPPort),
		Username:    strOr(tenant.SMTPUsername),
		FromAddress: strOr(tenant.SMTPFromAddress),
		Configured:  tenant.SMTPHost != nil,
		Error:       errMsg,
		Saved:       saved,
	}
}

func strOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intOr(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func handleUpdateMailConfigFragment(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())

		host := r.FormValue("host")
		username := r.FormValue("username")
		fromAddress := r.FormValue("from_address")
		password := r.FormValue("password")
		port, portErr := strconv.Atoi(r.FormValue("port"))

		if host == "" || username == "" || fromAddress == "" || portErr != nil || port == 0 {
			renderMailConfigResult(w, r, store, actor, "host, port, username, and from_address are required", false)
			return
		}

		cfg := domain.TenantMailConfig{
			Host:        host,
			Port:        port,
			Username:    username,
			Password:    password,
			FromAddress: fromAddress,
		}
		if err := store.Tenants().UpdateMailConfig(r.Context(), actor.TenantID, cfg); err != nil {
			renderMailConfigResult(w, r, store, actor, "internal error", false)
			return
		}
		renderMailConfigResult(w, r, store, actor, "", true)
	}
}

func renderMailConfigResult(w http.ResponseWriter, r *http.Request, store *postgres.Store, actor *domain.User, errMsg string, saved bool) {
	tenant, err := store.Tenants().GetByID(r.Context(), actor.TenantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderFragment(w, mailConfigFragment, "mail_config_form", mailConfigDataFromTenant(tenant, baseData{}, errMsg, saved))
}
