package web

import "github.com/mkandel/go-checklists/internal/domain"

// baseData carries fields the shared layout needs (e.g. nav rendering) on
// every full-page response. Page-specific data structs embed it so the
// layout can read .Actor regardless of which page produced the data.
type baseData struct {
	Actor *domain.User
}
