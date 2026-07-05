package web

import (
	"encoding/json"
	"net/http"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

var (
	templatesListTemplate  = pageTemplate("templates_list.html")
	templateDetailTemplate = pageTemplate("template_detail.html")
	templateNewTemplate    = pageTemplate("template_new.html")
)

// registerTemplateUIRoutes wires the templates browsing pages and the
// admin-only new-version builder. Reads are available to any signed-in user
// (mirroring internal/api's templates endpoints); creating a version is
// admin-only.
func registerTemplateUIRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.HandleFunc("GET /templates", requireAuthPage(handleTemplatesListPage(store)))
	mux.HandleFunc("GET /templates/new", requireAdminPage(handleTemplateNewPage()))
	mux.HandleFunc("GET /templates/{id}", requireAuthPage(handleTemplateDetailPage(store)))
	mux.Handle("POST /templates", api.RequireAuth(api.RequireAdmin(handleCreateTemplateVersion(store))))
}

// templateGroup collects every version of a same-named template, newest
// last, for the list page's per-name grouping.
type templateGroup struct {
	Latest   domain.Template
	Versions []domain.Template
}

type templatesListPageData struct {
	baseData
	IsAdmin bool
	Groups  []templateGroup
}

// groupTemplatesByName collects List's ascending-by-version rows into one
// templateGroup per name, taking the last-seen (highest-version) row as
// Latest. Shared by the templates list page and the checklist-create page's
// template picker.
func groupTemplatesByName(templates []domain.Template) []templateGroup {
	var groups []templateGroup
	byName := make(map[string]int)
	for _, t := range templates {
		if idx, ok := byName[t.Name]; ok {
			groups[idx].Versions = append(groups[idx].Versions, t)
			groups[idx].Latest = t
			continue
		}
		byName[t.Name] = len(groups)
		groups = append(groups, templateGroup{Latest: t, Versions: []domain.Template{t}})
	}
	return groups
}

func handleTemplatesListPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		templates, err := store.Templates().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		renderPage(w, templatesListTemplate, templatesListPageData{
			baseData: baseData{Actor: actor},
			IsAdmin:  actor.IsAdmin,
			Groups:   groupTemplatesByName(templates),
		})
	}
}

type templateDetailPageData struct {
	baseData
	Template      domain.Template
	Items         []domain.TemplateItem
	OtherVersions []domain.Template
}

func handleTemplateDetailPage(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathInt64(r, "id")
		if !ok {
			http.Error(w, "invalid template id", http.StatusBadRequest)
			return
		}
		actor, _ := api.UserFromContext(r.Context())
		t, items, err := store.Templates().Get(r.Context(), actor.TenantID, id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		all, err := store.Templates().List(r.Context(), actor.TenantID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var others []domain.Template
		for _, other := range all {
			if other.Name == t.Name && other.ID != t.ID {
				others = append(others, other)
			}
		}

		renderPage(w, templateDetailTemplate, templateDetailPageData{
			baseData:      baseData{Actor: actor},
			Template:      *t,
			Items:         items,
			OtherVersions: others,
		})
	}
}

func handleTemplateNewPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		renderPage(w, templateNewTemplate, templatesListPageData{baseData: baseData{Actor: actor}})
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

// handleCreateTemplateVersion mirrors internal/api's identical JSON handler
// (a small, easy-to-duplicate amount of logic, per this project's established
// preference for keeping internal/web decoupled from internal/api's
// internals) — called via fetch() from template_new.html's drag-drop item
// builder rather than a plain form post, since the item list is a dynamic
// JSON array.
func handleCreateTemplateVersion(store *postgres.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTemplateVersionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Name == "" || len(req.Items) == 0 {
			http.Error(w, "name and at least one item are required", http.StatusBadRequest)
			return
		}

		actor, _ := api.UserFromContext(r.Context())
		t := &domain.Template{TenantID: actor.TenantID, Name: req.Name}
		items := make([]domain.TemplateItem, len(req.Items))
		for i, it := range req.Items {
			items[i] = domain.TemplateItem{Name: it.Name, Position: i, ValidationRef: it.ValidationRef}
		}

		err := store.WithTx(r.Context(), func(tx *postgres.Store) error {
			return tx.Templates().CreateVersion(r.Context(), t, items)
		})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(struct {
			ID int64 `json:"id"`
		}{ID: t.ID})
	}
}
