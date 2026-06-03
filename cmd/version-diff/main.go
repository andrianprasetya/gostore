// Command version-diff generates openapi-v1.json and a breaking openapi-v2.json,
// then runs open-swag-go's versioning differ. Fase 7.
//
// v2 introduces several breaking changes:
//   - removes the /showcase endpoint            (endpoint_removed)   -> DETECTED
//   - adds a required field to POST /products   (required_field_added) -> DETECTED
//   - adds a required query param to POST /products (new required param) -> DETECTED
//   - removes the 404 response from GET /products/{id} (response_removed) -> DETECTED
//   - changes POST /products price from number to an object (type change) -> NOT DETECTED
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gostore/internal/handlers"

	openswag "github.com/gopackx/open-swag-go"
	"github.com/gopackx/open-swag-go/pkg/versioning"
)

// v2 request body: price becomes an object + a new required "currency" field.
type money struct {
	Amount   int    `json:"amount" swagger:"required"`
	Currency string `json:"currency" swagger:"required"`
}
type createProductRequestV2 struct {
	Name     string `json:"name" swagger:"required"`
	SKU      string `json:"sku" swagger:"required"`
	Price    money  `json:"price" swagger:"required"` // was number -> now object (type change)
	Currency string `json:"currency" swagger:"required"` // NEW required field
	Stock    int64  `json:"stock"`
}

func baseConfig() openswag.Config {
	return openswag.Config{
		Info: openswag.Info{Title: "GoStore API", Version: "1.0.0"},
		UI:   openswag.UIConfig{Theme: "purple"},
	}
}

func writeSpec(path string, endpoints []openswag.Endpoint, version string) {
	cfg := baseConfig()
	cfg.Info.Version = version
	d := openswag.New(cfg)
	d.AddAll(endpoints...)
	b, err := d.SpecJSON()
	if err != nil {
		log.Fatalf("spec %s: %v", path, err)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	log.Printf("wrote %s (%d bytes)", path, len(b))
}

func v2Endpoints() []openswag.Endpoint {
	var out []openswag.Endpoint
	for _, ep := range handlers.Endpoints {
		switch {
		case ep.Path == "/showcase":
			continue // endpoint removed
		case ep.Path == "/products" && ep.Method == "POST":
			ep.RequestBody = &openswag.RequestBody{Description: "Product v2", Required: true, Schema: createProductRequestV2{}}
			ep.Parameters = append(ep.Parameters, openswag.Parameter{Name: "reason", In: "query", Required: true, Description: "audit reason (new required)"})
			out = append(out, ep)
		case ep.Path == "/products/{id}" && ep.Method == "GET":
			ep.Responses = map[int]openswag.Response{200: ep.Responses[200]} // drop 404
			out = append(out, ep)
		default:
			out = append(out, ep)
		}
	}
	return out
}

func main() {
	writeSpec("spec/openapi-v1.json", handlers.Endpoints, "1.0.0")
	writeSpec("spec/openapi-v2.json", v2Endpoints(), "2.0.0")

	differ := versioning.NewDiffer()
	diff, err := differ.CompareFiles("spec/openapi-v1.json", "spec/openapi-v2.json")
	if err != nil {
		log.Fatalf("compare: %v", err)
	}

	fmt.Printf("\nBreaking changes: %v\n", diff.HasBreakingChanges())
	fmt.Printf("Summary: added=%d removed=%d modified=%d breaking=%d\n",
		diff.Summary.AddedEndpoints, diff.Summary.RemovedEndpoints,
		diff.Summary.ModifiedEndpoints, diff.Summary.BreakingChanges)

	fmt.Println("\n-- Breaking detail --")
	for _, b := range diff.Breaking {
		fmt.Printf("  [%s %s] %s\n      migration: %s\n", b.Method, b.Path, b.Reason, b.Migration)
	}

	fmt.Println("\n-- Changelog (markdown) --")
	fmt.Println(versioning.GenerateChangelog(diff))

	fmt.Println("-- NOTE (finding) --")
	fmt.Println("  The price number->object change on POST /products is a RESPONSE/BODY field")
	fmt.Println("  TYPE change; the differ only checks required-fields/response-codes/params,")
	fmt.Println("  so this type change is NOT reported. See FINDINGS.")

	if !diff.HasBreakingChanges() {
		log.Fatal("expected breaking changes but found none")
	}
}
