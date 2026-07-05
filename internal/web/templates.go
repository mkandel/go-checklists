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

// funcMap is the shared html/template function map available to every page.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"appName": func() string { return AppName },
		"version": func() string { return Version },
	}
}
