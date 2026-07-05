package web

import "html/template"

// AppName is the product's user-facing display name. Defined once here so
// it's easy to change — every template references it via {{appName}}
// rather than hardcoding the name.
const AppName = "ChecklistHQ"

// Version is the running build's version string, rendered in the page
// footer via {{version}}. Set by cmd/checklists-server/main.go at startup;
// defaults to "dev" for ad-hoc `go run`/tests that never set it.
var Version = "dev"

// NotificationsEnabled gates both RegisterRoutes mounting the notification
// endpoints and layout.html rendering the nav badge/SSE script. Set by
// cmd/checklists-server/main.go at startup from
// config.Config.NotificationsEnabled; defaults to false. The underlying
// templates and handlers are left in place so this can be flipped back on
// later.
var NotificationsEnabled = false

// funcMap is the shared html/template function map available to every page.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"appName":              func() string { return AppName },
		"version":              func() string { return Version },
		"notificationsEnabled": func() bool { return NotificationsEnabled },
	}
}
