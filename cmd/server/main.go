// Command server is GoStore's main Gin application: migrate -> seed -> serve,
// with go-audit capturing every data change and API call.
package main

import (
	"context"
	"log"
	"os"

	"gostore/internal/audit"
	"gostore/internal/bootstrap"
	"gostore/internal/db"
	"gostore/internal/handlers"
	"gostore/internal/router"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)

	conn, err := db.Open()
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer conn.Close()

	// Fresh schema + seed on every boot unless GOSTORE_NO_RESET=1.
	if os.Getenv("GOSTORE_NO_RESET") == "1" {
		if err := bootstrap.Migrate(conn.SQL); err != nil {
			log.Fatalf("migrate: %v", err)
		}
	} else {
		if err := bootstrap.Fresh(conn.SQL); err != nil {
			log.Fatalf("fresh migrate: %v", err)
		}
		if err := bootstrap.Seed(conn.SQL); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}

	ctx := context.Background()
	auditor, err := audit.Setup(ctx, conn.Gorm, conn.SQL)
	if err != nil {
		log.Fatalf("audit setup: %v", err)
	}

	if _, err := bootstrap.EnsureAdmin(conn.Gorm); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}

	apiKey := os.Getenv("GOSTORE_ADMIN_API_KEY")
	if apiKey == "" {
		apiKey = "gostore-admin-key"
	}

	h := &handlers.Handler{
		DB:          conn.Gorm,
		Auditor:     auditor,
		AdminAPIKey: apiKey,
	}

	r := router.New(h, conn.Gorm, nil) // docs mounted in Fase 4

	addr := os.Getenv("GOSTORE_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("[server] GoStore listening on %s (admin: %s / %s)", addr, bootstrap.AdminEmail, bootstrap.AdminPassword)
	if err := r.Run(addr); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
