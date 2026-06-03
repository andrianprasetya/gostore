// Package bootstrap centralizes "migrate then seed" so the server and the demo
// commands share one code path.
package bootstrap

import (
	"database/sql"
	"fmt"

	"gostore/internal/migrations"
	"gostore/internal/seed"

	"github.com/gopackx/go-migration/pkg/migrator"
	"github.com/gopackx/go-migration/pkg/schema/grammars"
	"github.com/gopackx/go-migration/pkg/seeder"
)

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
