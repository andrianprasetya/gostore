// Command smoke is the Fase 0 smoke test: boot Postgres (embedded or
// DATABASE_URL), open GORM, share the pool, and ping.
package main

import (
	"fmt"
	"log"

	"gostore/internal/db"
)

func main() {
	conn, err := db.Open()
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer conn.Close()

	var version string
	if err := conn.SQL.QueryRow("SELECT version()").Scan(&version); err != nil {
		log.Fatalf("select version: %v", err)
	}

	fmt.Println("[smoke] OK")
	fmt.Println("[smoke] DSN:", conn.DSN)
	fmt.Println("[smoke] server:", version)
}
