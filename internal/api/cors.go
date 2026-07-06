package api

import "net/http"

// AllowedOrigin is the single browser origin permitted to make cross-origin
// requests to this API — the web server's own origin, since the React/Qwik
// SPAs are served from there while calling this API on its own port. Set by
// cmd/checklists-server/main.go at startup from config.Config.WebOrigin();
// left empty (no CORS headers at all) for ad-hoc `go run`/tests, matching
// TrustProxy and NotificationsEnabled's zero-value-is-safe convention.
//
// Deployments fronted by Caddy (see Caddyfile) route /api/* and everything
// else through the same public origin, so the browser never actually makes
// a cross-origin request in that setup — this middleware only matters when
// something calls the API port directly (local dev without Caddy running).
var AllowedOrigin = ""

// WithCORS allows AllowedOrigin (if set) to make credentialed cross-origin
// requests to next, including the preflight OPTIONS request the browser
// sends ahead of any request carrying the X-CSRF-Token header. A single
// fixed allowed origin — rather than reflecting back whatever Origin header
// the caller sent — is deliberate: echoing an arbitrary Origin back with
// Allow-Credentials: true would let any site read this API on a logged-in
// user's behalf.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AllowedOrigin == "" {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == AllowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", AllowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+csrfHeaderName)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
