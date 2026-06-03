// Command server-chi mounts the SAME open-swag-go docs via the Chi adapter to
// prove adapter parity (Fase 4). It only serves the docs + spec (no DB), so the
// served /docs-chi/openapi.json can be diffed against the Gin server's spec.
package main

import (
	"log"
	"net/http"
	"os"

	"gostore/internal/docs"

	chiadapter "github.com/gopackx/open-swag-go/adapters/chi"
	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()
	chiadapter.Mount(r, docs.New(), "/docs-chi")

	addr := os.Getenv("GOSTORE_CHI_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	log.Printf("[server-chi] docs at http://localhost%s/docs-chi/", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
