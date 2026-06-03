// Package audit wires go-audit into GoStore: an Auditor over the shared
// *sql.DB and the GORM plugin for automatic data-change capture.
package audit

import (
	"context"
	"database/sql"

	goaudit "github.com/gopackx/go-audit"
	auditgorm "github.com/gopackx/go-audit/adapters/gorm"
	"gorm.io/gorm"
)

// ctxUserKey carries the acting user identity through context for UserFunc.
type ctxUserKey struct{}

type userIdentity struct {
	ID   string
	Type string
}

// WithUser attaches the acting user (id + type) to ctx.
func WithUser(ctx context.Context, id, typ string) context.Context {
	return context.WithValue(ctx, ctxUserKey{}, userIdentity{ID: id, Type: typ})
}

func userFromContext(ctx context.Context) (string, string) {
	if u, ok := ctx.Value(ctxUserKey{}).(userIdentity); ok {
		return u.ID, u.Type
	}
	return "", ""
}

// ExcludedEntities are tables kept out of the data-change audit trail: the
// audit/infra tables and the notification table (so audit does not "swallow"
// the notification package's GORM writes). See FINDINGS "Integrasi".
var ExcludedEntities = []string{"notifications", "audit_logs", "audit_api_logs", "migrations"}

// Setup builds the auditor over sqlDB, runs AutoMigrate (creates audit_logs +
// audit_api_logs), and attaches the GORM plugin to gormDB for automatic
// create/update/delete capture.
func Setup(ctx context.Context, gormDB *gorm.DB, sqlDB *sql.DB) (goaudit.Auditor, error) {
	auditor, err := goaudit.New(sqlDB, goaudit.Config{
		Dialect:  goaudit.PostgreSQL,
		UserFunc: userFromContext,
		DataAudit: goaudit.DataAuditConfig{
			Enabled:         true,
			ExcludeFields:   []string{"password", "remember_token"},
			ExcludeEntities: ExcludedEntities,
		},
		APIAudit: goaudit.APIAuditConfig{
			Enabled:          true,
			RedactHeaders:    []string{"Authorization", "X-API-Key"},
			RedactBodyFields: []string{"password", "secret", "card_number", "cvv"},
			MaxBodySize:      2048,
		},
	})
	if err != nil {
		return nil, err
	}
	if err := auditor.AutoMigrate(ctx); err != nil {
		return nil, err
	}
	if err := gormDB.Use(auditgorm.Plugin(auditor)); err != nil {
		return nil, err
	}
	return auditor, nil
}
