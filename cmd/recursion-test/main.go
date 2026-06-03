// Command recursion-test isolates the open-swag-go schema generator's lack of a
// visited-set: a self-referential type (dto.Category) makes fromReflectType
// recurse forever. Run with a timeout; it stack-overflows (fatal, unrecoverable).
package main

import (
	"fmt"

	"gostore/internal/dto"

	"github.com/gopackx/open-swag-go/pkg/schema"
)

func main() {
	fmt.Println("generating schema for self-referential dto.Category ...")
	s := schema.FromType(dto.Category{}) // expected: runtime: goroutine stack exceeds limit
	fmt.Printf("unexpectedly survived: %+v\n", s)
}
