package api

import (
	"encoding/json"
	"net/http"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// registerTemplateRoutes wires the template endpoints into mux. Reads are
// available to any signed-in user (to pick a template when creating a
// checklist); creating a new version is admin-only.
func registerTemplateRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /templates", RequireAuth(handleListTemplates(store)))
	mux.Handle("GET /templates/{id}", RequireAuth(handleGetTemplate(store)))
	mux.Handle("GET /templates/latest/{name}", RequireAuth(handleGetLatestTemplate(store)))
	mux.Handle("POST /templates", RequireAuth(RequireAdmin(handleCreateTemplateVersion(store))))
}

// templateResponse combines a Template with its items into one JSON object,
// since TemplateRepo.Get/GetLatestByName return them as separate values.
type templateResponse struct {
	domain.Template
	Items []domain.TemplateItem `json:"items"`
}

func handleListTemplates(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := UserFromContext(r.Context())
		templates, err := store.Templates().List(r.Context(), actor.TenantID)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, templates)
	}
}

func handleGetTemplate(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid template id", http.StatusBadRequest)
			return
		}
		actor, _ := UserFromContext(r.Context())
		t, items, err := store.Templates().Get(r.Context(), actor.TenantID, id)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, templateResponse{Template: *t, Items: items})
	}
}

func handleGetLatestTemplate(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		actor, _ := UserFromContext(r.Context())
		t, items, err := store.Templates().GetLatestByName(r.Context(), actor.TenantID, name)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, templateResponse{Template: *t, Items: items})
	}
}

type createTemplateVersionRequest struct {
	Name  string                      `json:"name"`
	Items []createTemplateVersionItem `json:"items"`
}

type createTemplateVersionItem struct {
	Name          string `json:"name"`
	ValidationRef string `json:"validation_ref"`
}

func handleCreateTemplateVersion(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTemplateVersionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		actor, _ := UserFromContext(r.Context())
		t := &domain.Template{TenantID: actor.TenantID, Name: req.Name}
		items := make([]domain.TemplateItem, len(req.Items))
		for i, it := range req.Items {
			items[i] = domain.TemplateItem{Name: it.Name, Position: i, ValidationRef: it.ValidationRef}
		}

		err := store.WithTx(r.Context(), func(tx *postgres.Store) error {
			return tx.Templates().CreateVersion(r.Context(), t, items)
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, templateResponse{Template: *t, Items: items})
	}
}
