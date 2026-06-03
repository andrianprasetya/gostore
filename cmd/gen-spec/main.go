// Command gen-spec exports the OpenAPI spec to a file without running the
// server. Usage: gen-spec [output-path]   (default spec/openapi.json)
package main

import (
	"log"
	"os"
	"path/filepath"

	"gostore/internal/docs"
)

func main() {
	out := "spec/openapi.json"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	spec, err := docs.New().SpecJSON()
	if err != nil {
		log.Fatalf("build spec: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(out, spec, 0o644); err != nil {
		log.Fatalf("write: %v", err)
	}
	log.Printf("wrote %s (%d bytes)", out, len(spec))
}
