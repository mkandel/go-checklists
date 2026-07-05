package api

import (
	"encoding/json"
	"net/http"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// registerTenantMailRoutes wires the admin-only per-tenant SMTP config
// endpoints into mux.
func registerTenantMailRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /api/admin/tenant/mail-config", RequireAuth(RequireAdmin(handleGetTenantMailConfig(store))))
	mux.Handle("PUT /api/admin/tenant/mail-config", RequireAuth(RequireAdmin(handleUpdateTenantMailConfig(store))))
}

// tenantMailConfigResponse is the read shape for a tenant's SMTP config.
// It never includes the password, mirroring domain.Tenant.SMTPPassword's
// json:"-" tag.
type tenantMailConfigResponse struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	FromAddress string `json:"from_address"`
	Configured  bool   `json:"configured"`
}

func handleGetTenantMailConfig(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := UserFromContext(r.Context())
		tenant, err := store.Tenants().GetByID(r.Context(), actor.TenantID)
		if err != nil {
			writeDomainError(w, err)
			return
		}

		resp := tenantMailConfigResponse{Configured: tenant.SMTPHost != nil}
		if tenant.SMTPHost != nil {
			resp.Host = *tenant.SMTPHost
		}
		if tenant.SMTPPort != nil {
			resp.Port = *tenant.SMTPPort
		}
		if tenant.SMTPUsername != nil {
			resp.Username = *tenant.SMTPUsername
		}
		if tenant.SMTPFromAddress != nil {
			resp.FromAddress = *tenant.SMTPFromAddress
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// updateTenantMailConfigRequest is the write shape for a tenant's SMTP
// config. Host/Port/Username/FromAddress are full-replace/required every
// call. Password is the one field where an empty string means "keep the
// existing password" — see domain.TenantMailConfig.
type updateTenantMailConfigRequest struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"from_address"`
}

func handleUpdateTenantMailConfig(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateTenantMailConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Host == "" || req.Port == 0 || req.Username == "" || req.FromAddress == "" {
			http.Error(w, "host, port, username, and from_address are required", http.StatusBadRequest)
			return
		}

		actor, _ := UserFromContext(r.Context())
		cfg := domain.TenantMailConfig{
			Host:        req.Host,
			Port:        req.Port,
			Username:    req.Username,
			Password:    req.Password,
			FromAddress: req.FromAddress,
		}
		if err := store.Tenants().UpdateMailConfig(r.Context(), actor.TenantID, cfg); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
