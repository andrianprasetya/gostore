// Package middleware provides GoStore's Gin middleware: a per-request
// transaction id (propagated into context for go-audit + go-notification) and
// bearer / API-key authentication.
package middleware

import (
	"net/http"
	"strings"

	"gostore/internal/audit"
	"gostore/internal/dto"
	"gostore/internal/models"

	goaudit "github.com/gopackx/go-audit"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ctxUserGin = "gostore_user"
	ctxTxGin   = "gostore_txid"
)

// Transaction generates a transaction id, exposes it as a response header, and
// stores it in the request context so audit data-changes, audit API-calls, and
// notifications in this request all correlate under one id.
func Transaction() gin.HandlerFunc {
	return func(c *gin.Context) {
		txID := goaudit.NewTransactionID()
		ctx := goaudit.WithTransactionID(c.Request.Context(), txID)
		c.Request = c.Request.WithContext(ctx)
		c.Set(ctxTxGin, txID)
		c.Header("X-Transaction-ID", txID)
		c.Next()
	}
}

// TxID returns the current request's transaction id.
func TxID(c *gin.Context) string {
	if v, ok := c.Get(ctxTxGin); ok {
		return v.(string)
	}
	return ""
}

// Bearer authenticates via "Authorization: Bearer <user-id>". On success it
// loads the user, stores it on the gin context, and enriches the request
// context with the acting user's identity for go-audit's UserFunc.
func Bearer(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		token = strings.TrimSpace(token)
		if token == "" {
			abort(c, http.StatusUnauthorized, "missing bearer token")
			return
		}
		var user models.User
		if err := db.WithContext(c.Request.Context()).Where("id = ?", token).First(&user).Error; err != nil {
			abort(c, http.StatusUnauthorized, "invalid token")
			return
		}
		setUser(c, &user)
		c.Next()
	}
}

// RequireAdmin must run after Bearer; it rejects non-admin users.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := User(c)
		if u == nil || u.Role != "admin" {
			abort(c, http.StatusForbidden, "admin role required")
			return
		}
		c.Next()
	}
}

// APIKey authenticates internal/admin endpoints via the X-API-Key header.
func APIKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-API-Key") != expected {
			abort(c, http.StatusUnauthorized, "invalid api key")
			return
		}
		// Tag the audit actor as the service account.
		c.Request = c.Request.WithContext(audit.WithUser(c.Request.Context(), "service", "apikey"))
		c.Next()
	}
}

func setUser(c *gin.Context, u *models.User) {
	c.Set(ctxUserGin, u)
	ctx := audit.WithUser(c.Request.Context(), itoa(u.ID), u.Role)
	c.Request = c.Request.WithContext(ctx)
}

// User returns the authenticated user, or nil.
func User(c *gin.Context) *models.User {
	if v, ok := c.Get(ctxUserGin); ok {
		return v.(*models.User)
	}
	return nil
}

func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, dto.ErrorResponse{Error: msg, Code: code})
}

func itoa(u uint64) string {
	// small helper to avoid importing strconv everywhere
	if u == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for u > 0 {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
	}
	return string(b[i:])
}
