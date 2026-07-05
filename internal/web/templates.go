package web

import "html/template"

// AppName is the product's user-facing display name. Defined once here so
// it's easy to change — every template references it via {{appName}}
// rather than hardcoding the name.
const AppName = "ChecklistHQ"

// funcMap is the shared html/template function map available to every page.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"appName": func() string { return AppName },
	}
}
