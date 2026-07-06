// Package webreact serves the built React SPA (see web-react/ at the repo
// root for its source) from an embedded static bundle. It doesn't touch the
// database or session store directly — the SPA authenticates and fetches
// data entirely by calling the unchanged JSON API under /api/*, the same
// way any other client of that API would.
package webreact

import (
	"embed"
	"io/fs"
	"net/http"
)

// distFS embeds whatever's in dist/ at compile time. A fresh checkout has
// only the placeholder .gitkeep (dist/'s real contents are gitignored build
// output — see scripts/build-frontends.ps1) so `go build ./...` always
// succeeds even before the frontend has ever been built; RegisterRoutes
// notices the missing index.html at request time and says so instead of
// serving a blank page.
//
//go:embed all:dist
var distFS embed.FS

// RegisterRoutes mounts the React SPA's static assets on mux, falling back
// to index.html for any path that isn't a real static file — the standard
// SPA pattern, so a client-side route like /checklists/42 loads the app
// shell instead of 404ing. Shared auth routes (/login, /register, /logout,
// /password-reset/*) are registered separately by
// cmd/checklists-server/main.go via api.RegisterAuthRoutes; the SPA calls
// those the same way it calls every other endpoint, via fetch().
func RegisterRoutes(mux *http.ServeMux) {
	assets, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("webreact: embedded dist directory missing: " + err.Error())
	}
	mux.Handle("GET /", spaHandler(assets))
}

// notBuiltMessage is served in place of the app whenever dist/index.html is
// missing — i.e. scripts/build-frontends.ps1 (or the equivalent npm build)
// hasn't been run yet against this checkout.
const notBuiltMessage = "React frontend not built yet. Run scripts/build-frontends.ps1 (or `cd web-react && npm ci && npm run build`), then restart the server.\n"

// spaHandler serves a static asset from assets if the request path matches
// one, or index.html (the app shell) otherwise. If index.html itself isn't
// present, it reports notBuiltMessage instead of a bare 404.
func spaHandler(assets fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(assets))
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(assets, "index.html"); err != nil {
			http.Error(w, notBuiltMessage, http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/" {
			if _, err := fs.Stat(assets, r.URL.Path[1:]); err != nil {
				r2 := new(http.Request)
				*r2 = *r
				r2.URL.Path = "/"
				fileServer.ServeHTTP(w, r2)
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	}
}
