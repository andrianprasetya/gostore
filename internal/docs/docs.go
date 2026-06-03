// Package docs assembles the open-swag-go documentation for GoStore and exposes
// helpers to serve it on either Gin or Chi (Fase 4: prove adapter parity).
package docs

import (
	"net/http"

	"gostore/internal/handlers"

	openswag "github.com/gopackx/open-swag-go"
	ginadapter "github.com/gopackx/open-swag-go/adapters/gin"
	"github.com/gin-gonic/gin"
)

// New builds the *Docs with GoStore's config + all co-located endpoints.
func New() *openswag.Docs {
	d := openswag.New(openswag.Config{
		Info: openswag.Info{
			Title:       "GoStore API",
			Version:     "1.0.0",
			Description: "Dogfooding the **gopackx** stack: migration + audit + notification + docs.",
			Contact:     &openswag.Contact{Name: "Andrian Prasetya", URL: "https://github.com/andrianprasetya"},
			License:     &openswag.License{Name: "MIT"},
		},
		Servers: []openswag.Server{{URL: "http://localhost:8080", Description: "Local"}},
		Tags: []openswag.Tag{
			{Name: "Auth", Description: "Registration & login"},
			{Name: "Products", Description: "Catalog"},
			{Name: "Orders", Description: "Checkout"},
			{Name: "Admin", Description: "Service-account endpoints"},
			{Name: "Showcase", Description: "Edge-type schema demo"},
		},
		UI: openswag.UIConfig{
			Theme: "purple", DarkMode: true, ShowSidebar: true, SidebarSearch: true,
			TagGrouping: true, CollapsibleSchemas: true,
		},
	})
	d.AddAll(handlers.Endpoints...)
	return d
}

// GinMount mounts the docs UI + spec at basePath on a Gin engine.
func GinMount(basePath string) func(*gin.Engine) {
	return func(r *gin.Engine) {
		ginadapter.Mount(r, New(), basePath)
	}
}

// NetHTTPMount mounts the docs on a net/http mux (used by the Chi server too,
// since Chi embeds an http.Handler-compatible router).
func NetHTTPMount(mux *http.ServeMux, basePath string) {
	New().Mount(mux, basePath)
}
