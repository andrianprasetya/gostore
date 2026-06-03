// Package bootstrap centralizes "migrate then seed" so the server and the demo
// commands share one code path.
package bootstrap

import (
	"database/sql"
	"fmt"

	"gostore/internal/idgen"
	"gostore/internal/migrations"
	"gostore/internal/models"
	"gostore/internal/seed"

	"github.com/gopackx/go-migration/pkg/migrator"
	"github.com/gopackx/go-migration/pkg/schema/grammars"
	"github.com/gopackx/go-migration/pkg/seeder"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AdminEmail / AdminPassword are the well-known demo admin credentials created
// by EnsureAdmin so admin-only endpoints are testable out of the box.
const (
	AdminEmail    = "admin@gostore.dev"
	AdminPassword = "admin123"
)

// EnsureAdmin creates the demo admin user if it does not already exist.
func EnsureAdmin(gdb *gorm.DB) (models.User, error) {
	var existing models.User
	if err := gdb.Where("email = ?", AdminEmail).First(&existing).Error; err == nil {
		return existing, nil
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(AdminPassword), bcrypt.DefaultCost)
	admin := models.User{
		UUID: idgen.NewUUID(), Name: "GoStore Admin",
		Email: AdminEmail, Password: string(hash), Role: "admin", Active: true,
	}
	if err := gdb.Create(&admin).Error; err != nil {
		return models.User{}, err
	}
	return admin, nil
}

// NewMigrator returns a migrator with all GoStore migrations registered.
func NewMigrator(sqlDB *sql.DB) (*migrator.Migrator, error) {
	m := migrator.New(sqlDB,
		migrator.WithGrammar(&grammars.PostgresGrammar{}),
		migrator.WithTableName("migrations"),
	)
	if err := migrations.Register(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Fresh drops all tables and re-runs every migration. Use for demos/dev.
func Fresh(sqlDB *sql.DB) error {
	m, err := NewMigrator(sqlDB)
	if err != nil {
		return err
	}
	return m.Fresh()
}

// Migrate runs pending migrations only (no drop). Use for the server.
func Migrate(sqlDB *sql.DB) error {
	m, err := NewMigrator(sqlDB)
	if err != nil {
		return err
	}
	return m.Up()
}

// Seed runs all seeders in dependency order.
func Seed(sqlDB *sql.DB) error {
	reg := seeder.NewRegistry()
	if err := seed.Register(reg); err != nil {
		return fmt.Errorf("register seeders: %w", err)
	}
	return seeder.NewRunner(reg, sqlDB, nil).RunAll()
}
