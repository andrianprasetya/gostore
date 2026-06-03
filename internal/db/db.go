// Package db bootstraps the PostgreSQL connection for GoStore.
//
// Strategy (see plan.md "Keputusan teknis" + FINDINGS env note):
//   - GORM opens the connection to Postgres.
//   - We pull the underlying *sql.DB via gormDB.DB() and hand the SAME pool to
//     go-migration (PostgresGrammar). One DB, one pool.
//   - If DATABASE_URL is set we use a real/remote Postgres. Otherwise we boot an
//     embedded Postgres (real binary, no Docker) so the whole thing is
//     reproducible on a machine without Docker.
package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Conn bundles everything callers need plus a shutdown hook.
type Conn struct {
	Gorm *gorm.DB
	SQL  *sql.DB
	DSN  string

	stop func() error // stops embedded postgres, if any
}

// Close shuts the pool and (if embedded) the Postgres process.
func (c *Conn) Close() error {
	if c.SQL != nil {
		_ = c.SQL.Close()
	}
	if c.stop != nil {
		return c.stop()
	}
	return nil
}

const embeddedDSN = "postgres://gostore:gostore@localhost:5432/gostore?sslmode=disable"

// Open returns a live connection. When DATABASE_URL is empty an embedded
// Postgres is started and its lifecycle is tied to Conn.Close().
func Open() (*Conn, error) {
	dsn := os.Getenv("DATABASE_URL")
	var stop func() error

	if dsn == "" {
		log.Println("[db] DATABASE_URL unset → starting embedded Postgres (no Docker)")
		ep, err := startEmbedded()
		if err != nil {
			return nil, fmt.Errorf("start embedded postgres: %w", err)
		}
		stop = ep.Stop
		dsn = embeddedDSN
	}

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		if stop != nil {
			_ = stop()
		}
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		if stop != nil {
			_ = stop()
		}
		return nil, fmt.Errorf("gorm.DB(): %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	if err := sqlDB.Ping(); err != nil {
		if stop != nil {
			_ = stop()
		}
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &Conn{Gorm: gormDB, SQL: sqlDB, DSN: dsn, stop: stop}, nil
}

func startEmbedded() (*embeddedpostgres.EmbeddedPostgres, error) {
	runtimePath := filepath.Join(os.TempDir(), "gostore-embedded-pg")
	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Username("gostore").
			Password("gostore").
			Database("gostore").
			Version(embeddedpostgres.V16).
			Port(5432).
			RuntimePath(runtimePath).
			StartTimeout(60 * time.Second),
	)
	if err := ep.Start(); err != nil {
		return nil, err
	}
	return ep, nil
}
