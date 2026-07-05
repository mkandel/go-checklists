// Package web serves the server-rendered browser UI (htmx + Alpine.js +
// SortableJS) alongside the JSON API in internal/api. Both front-ends
// register onto the same *http.ServeMux and are wrapped exactly once by
// api.WithSession — see cmd/checklists-server/main.go.
package web

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mkandel/go-checklists/internal/api"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

//go:embed templates/*.html templates/partials/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// baseTemplate holds the shared layout, parsed once. Each page clones it via
// pageTemplate rather than sharing a single tree, so that html/template's
// named-block definitions (title, content) don't collide across pages.
var baseTemplate = template.Must(template.New("layout").Funcs(funcMap()).ParseFS(templatesFS, "templates/layout.html"))

// pageTemplate clones baseTemplate and parses the named page-specific files
// (relative to templates/) into the clone. Most pages pass a single content
// file; pages that reuse a fragment's block (e.g. a table shared with an
// htmx-swapped response) pass that fragment's file too.
func pageTemplate(names ...string) *template.Template {
	t := template.Must(baseTemplate.Clone())
	return template.Must(t.ParseFS(templatesFS, prefixTemplateNames(names)...))
}

// fragmentTemplate parses the named files (relative to templates/) into a
// fresh tree with no layout, for handlers that render just a partial (an
// htmx-swapped fragment) rather than a full page.
func fragmentTemplate(names ...string) *template.Template {
	t := template.New("fragment").Funcs(funcMap())
	return template.Must(t.ParseFS(templatesFS, prefixTemplateNames(names)...))
}

func prefixTemplateNames(names []string) []string {
	files := make([]string, len(names))
	for i, n := range names {
		files[i] = "templates/" + n
	}
	return files
}

// renderPage executes t's "layout" template with data as the full HTML
// response.
func renderPage(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// renderFragment executes t's named block with data, for htmx-swapped
// partial responses.
func renderFragment(w http.ResponseWriter, t *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// requireAuthPage redirects to /login (preserving the original path as
// ?next=) instead of the plain-body 401 api.RequireAuth gives fragment/JSON
// endpoints, which is wrong for a page a browser navigates to directly.
func requireAuthPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := api.UserFromContext(r.Context()); !ok {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// requireAdminPage is requireAuthPage plus an admin check, 403ing (not
// redirecting) a signed-in non-admin.
func requireAdminPage(next http.HandlerFunc) http.HandlerFunc {
	return requireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		actor, _ := api.UserFromContext(r.Context())
		if !actor.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// pathInt64 parses the named path value as an int64, mirroring
// internal/api's identical unexported helper.
func pathInt64(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// RegisterRoutes wires the UI's page and fragment routes onto mux.
func RegisterRoutes(mux *http.ServeMux, store *postgres.Store) {
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))
	registerAuthRoutes(mux, store)
	registerGroupRoutes(mux, store)
	registerAdminUserRoutes(mux, store)
	registerAdminMailRoutes(mux, store)
	registerAdminChecklistPolicyRoutes(mux, store)
	registerNotificationRoutes(mux, store)
	registerTemplateUIRoutes(mux, store)
	registerChecklistUIRoutes(mux, store)
	registerChecklistDetailRoutes(mux, store)
}
