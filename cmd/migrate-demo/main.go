// Command migrate-demo exercises the full go-migration API surface against a
// live Postgres (embedded or DATABASE_URL) and prints a coverage checklist.
// This is the Fase 1 verification harness.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"gostore/internal/db"
	"gostore/internal/migrations"
	"gostore/internal/seed"

	"github.com/gopackx/go-migration/pkg/migrator"
	"github.com/gopackx/go-migration/pkg/schema"
	"github.com/gopackx/go-migration/pkg/schema/grammars"
	"github.com/gopackx/go-migration/pkg/seeder"
)

var checks []string

func check(name string, ok bool, detail string) {
	mark := "PASS"
	if !ok {
		mark = "FAIL"
	}
	line := fmt.Sprintf("[%s] %s — %s", mark, name, detail)
	checks = append(checks, line)
	fmt.Println(line)
}

func main() {
	conn, err := db.Open()
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer conn.Close()
	sqlDB := conn.SQL

	// Clean slate: drop everything from previous runs.
	if _, err := sqlDB.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		log.Fatalf("reset schema: %v", err)
	}

	newMigrator := func() *migrator.Migrator {
		m := migrator.New(sqlDB,
			migrator.WithGrammar(&grammars.PostgresGrammar{}),
			migrator.WithTableName("migrations"),
		)
		if err := migrations.Register(m); err != nil {
			log.Fatalf("register migrations: %v", err)
		}
		// Hooks: log duration.
		m.BeforeMigrate(func(name, dir string) error { return nil })
		m.AfterMigrate(func(name, dir string, d time.Duration) error {
			fmt.Printf("    · %s (%s) in %v\n", name, dir, d)
			return nil
		})
		return m
	}

	// --- Up ---
	m := newMigrator()
	if err := m.Up(); err != nil {
		log.Fatalf("Up: %v", err)
	}
	check("Up runs all migrations", tableExists(sqlDB, "order_items"), "order_items table created")

	// Edge: regular index (separate CREATE INDEX statement in one Exec).
	check("Regular Index (multi-statement Exec)", indexExists(sqlDB, "idx_products_name"),
		"idx_products_name present")
	check("Combined UniqueIndex", indexExists(sqlDB, "uniq_order_items_order_id_product_id"),
		"unique (order_id, product_id) present")

	// Edge: Alter add + rename + drop.
	b := schema.NewBuilder(sqlDB, &grammars.PostgresGrammar{})
	hasBarcode, _ := b.HasColumn("products", "barcode")
	hasDetails, _ := b.HasColumn("products", "details")
	hasOldDesc, _ := b.HasColumn("products", "description")
	hasTemp, _ := b.HasColumn("products", "temp_flag")
	check("Alter: add column", hasBarcode, "products.barcode added")
	check("Alter: rename column", hasDetails && !hasOldDesc, "description -> details")
	check("Alter: drop column", !hasTemp, "temp_flag dropped")

	// Edge: JSON -> jsonb, UUID native, Unsigned ignored.
	check("JSON maps to jsonb", colType(sqlDB, "users", "preferences") == "jsonb",
		"users.preferences is "+colType(sqlDB, "users", "preferences"))
	check("UUID native type", colType(sqlDB, "users", "uuid") == "uuid",
		"users.uuid is "+colType(sqlDB, "users", "uuid"))
	check("Unsigned() ignored on PG (no error)", colType(sqlDB, "users", "credit") == "bigint",
		"users.credit is "+colType(sqlDB, "users", "credit"))

	// HasTable / HasColumn.
	ht, _ := b.HasTable("orders")
	htMissing, _ := b.HasTable("nope_table")
	check("HasTable", ht && !htMissing, "orders=true, nope_table=false")

	// --- Status ---
	st, err := m.Status()
	if err != nil {
		log.Fatalf("Status: %v", err)
	}
	appliedCount := 0
	for _, s := range st {
		if s.Applied {
			appliedCount++
		}
	}
	check("Status reports applied", appliedCount == 5, fmt.Sprintf("%d/5 applied, batch=%d", appliedCount, st[0].Batch))

	// --- Rollback(0): last batch (all 5, since one Up = one batch) ---
	if err := m.Rollback(0); err != nil {
		log.Fatalf("Rollback(0): %v", err)
	}
	check("Rollback(0) rolls back batch", !tableExists(sqlDB, "users"),
		"all tables from batch 1 dropped")

	// Re-up in two batches to test step rollback + batch tracking.
	m2 := newMigrator()
	// Batch 2: register only does all; emulate two batches by Up twice is not
	// possible (Up applies all pending at once). Instead verify Rollback(N step).
	if err := m2.Up(); err != nil {
		log.Fatalf("re-Up: %v", err)
	}
	if err := m2.Rollback(2); err != nil {
		log.Fatalf("Rollback(2): %v", err)
	}
	// Last 2 by name are 000004 and 000005 -> order_items + alter rolled back.
	check("Rollback(2) step rollback", !tableExists(sqlDB, "order_items") && tableExists(sqlDB, "orders"),
		"order_items + alter down; orders still present")
	st2, _ := m2.Status()
	pending := 0
	for _, s := range st2 {
		if !s.Applied {
			pending++
		}
	}
	check("Batch tracking after step rollback", pending == 2, fmt.Sprintf("%d pending", pending))

	// --- Reset ---
	if err := m2.Reset(); err != nil {
		log.Fatalf("Reset: %v", err)
	}
	check("Reset drops everything", !tableExists(sqlDB, "products"), "no app tables")

	// --- Refresh (Reset + Up) ---
	m3 := newMigrator()
	if err := m3.Refresh(); err != nil {
		log.Fatalf("Refresh: %v", err)
	}
	check("Refresh re-applies", tableExists(sqlDB, "order_items"), "order_items back")

	// --- Fresh (drop all tables + Up) ---
	if err := m3.Fresh(); err != nil {
		log.Fatalf("Fresh: %v", err)
	}
	check("Fresh re-applies from clean schema", tableExists(sqlDB, "order_items"), "order_items back after Fresh")

	// --- Seeders (dependency order) ---
	reg := seeder.NewRegistry()
	if err := seed.Register(reg); err != nil {
		log.Fatalf("seed.Register: %v", err)
	}
	runner := seeder.NewRunner(reg, sqlDB, nil)
	if err := runner.RunAll(); err != nil {
		log.Fatalf("RunAll seeders: %v", err)
	}
	check("Seeder dependency order (OrderSeeder after deps)", countRows(sqlDB, "orders") > 0,
		fmt.Sprintf("users=%d products=%d orders=%d items=%d",
			countRows(sqlDB, "users"), countRows(sqlDB, "products"),
			countRows(sqlDB, "orders"), countRows(sqlDB, "order_items")))

	// --- Circular dependency detection ---
	creg := seeder.NewRegistry()
	_ = creg.Register("CircularA", &seed.CircularA{})
	_ = creg.Register("CircularB", &seed.CircularB{})
	cerr := seeder.NewRunner(creg, sqlDB, nil).RunAll()
	check("Circular dependency detected", cerr != nil, fmt.Sprintf("err=%v", cerr))

	// --- Factory: Make / MakeMany / State / WithState ---
	f := seed.UserFactory()
	one := f.Make()
	many := f.MakeMany(20)
	admin := f.WithState("admin").Make()
	check("Factory Make", one.Name != "", "single user: "+one.Name)
	check("Factory MakeMany(20)", len(many) == 20, fmt.Sprintf("%d users", len(many)))
	check("Factory WithState(admin)", admin.Role == "admin", "admin role + name="+admin.Name)

	// --- DisableTransaction: opt-out really changes behaviour ---
	disableTxExperiment(sqlDB)

	// --- Summary ---
	fmt.Println("\n================ Fase 1 coverage ================")
	fail := 0
	for _, c := range checks {
		if c[1:5] == "FAIL" {
			fail++
		}
	}
	fmt.Printf("Total: %d checks, %d failed\n", len(checks), fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// disableTxExperiment verifies that a failing migration WITH a transaction
// rolls back cleanly, while DisableTransaction() leaves partial DDL behind —
// the real behavioural difference of the opt-out on Postgres.
func disableTxExperiment(sqlDB *sql.DB) {
	_, _ = sqlDB.Exec(`DROP TABLE IF EXISTS exp_a, exp_b`)

	// Migration that creates exp_a successfully then fails on a bad statement.
	runFailing := func(disable bool) {
		m := migrator.New(sqlDB, migrator.WithGrammar(&grammars.PostgresGrammar{}),
			migrator.WithTableName("migrations"))
		name := "20260101000010_failing_txn"
		if disable {
			name = "20260101000011_failing_notxn"
			_ = m.Register(name, &failingMigration{disable: true})
		} else {
			_ = m.Register(name, &failingMigration{disable: false})
		}
		_ = m.Up() // expected to error
	}

	// With transaction: exp_a must NOT exist after failure (rolled back).
	runFailing(false)
	withTxLeftBehind := tableExists(sqlDB, "exp_a")
	_, _ = sqlDB.Exec(`DROP TABLE IF EXISTS exp_a, exp_b`)

	// Without transaction: exp_b must exist after failure (partial, no rollback).
	runFailing(true)
	noTxLeftBehind := tableExists(sqlDB, "exp_b")
	_, _ = sqlDB.Exec(`DROP TABLE IF EXISTS exp_a, exp_b`)

	check("DisableTransaction opt-out differs",
		!withTxLeftBehind && noTxLeftBehind,
		fmt.Sprintf("tx-wrapped rolled back exp_a=%v; no-tx kept exp_b=%v", withTxLeftBehind, noTxLeftBehind))
}

// failingMigration creates a table, then runs an invalid statement to force an
// error mid-migration.
type failingMigration struct{ disable bool }

func (f *failingMigration) DisableTransaction() bool { return f.disable }

func (f *failingMigration) Up(s *schema.Builder) error {
	tbl := "exp_a"
	if f.disable {
		tbl = "exp_b"
	}
	if err := s.Create(tbl, func(bp *schema.Blueprint) { bp.ID() }); err != nil {
		return err
	}
	// Force a failure with raw invalid DDL.
	return s.RawExec("CREATE TABLE this_is_invalid (")
}

func (f *failingMigration) Down(s *schema.Builder) error {
	return errors.New("not reversible")
}

// --- introspection helpers ---

func tableExists(db *sql.DB, name string) bool {
	var n int
	_ = db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, name,
	).Scan(&n)
	return n > 0
}

func indexExists(db *sql.DB, name string) bool {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname='public' AND indexname=$1`, name).Scan(&n)
	return n > 0
}

func colType(db *sql.DB, table, col string) string {
	var t string
	_ = db.QueryRow(
		`SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`,
		table, col,
	).Scan(&t)
	return t
}

func countRows(db *sql.DB, table string) int {
	var n int
	_ = db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table)).Scan(&n)
	return n
}
