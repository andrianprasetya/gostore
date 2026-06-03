// Package router wires GoStore's Gin routes and middleware.
package router

import (
	"gostore/internal/handlers"
	"gostore/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// New builds the Gin engine. docsMount, when non-nil, is called to mount the
// open-swag-go UI (Fase 4) so this package stays decoupled from the docs setup.
func New(h *handlers.Handler, db *gorm.DB, docsMount func(*gin.Engine)) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), middleware.Transaction())

	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	r.GET("/products", h.ListProducts)
	r.GET("/products/:id", h.GetProduct)
	r.GET("/showcase", h.Showcase)

	auth := r.Group("/", middleware.Bearer(db))
	auth.POST("/products", middleware.RequireAdmin(), h.CreateProduct)
	auth.POST("/products/:id/image", h.UploadProductImage)
	auth.POST("/orders", h.CreateOrder)
	auth.GET("/orders/:id", h.GetOrder)

	admin := r.Group("/admin", middleware.APIKey(h.AdminAPIKey))
	admin.GET("/orders", h.AdminListOrders)

	if docsMount != nil {
		docsMount(r)
	}
	return r
}
