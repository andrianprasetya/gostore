// Command audit-demo exercises go-audit end-to-end against live Postgres:
// auto-capture (create/update/delete), ExcludeFields, soft delete, API logging
// with redaction/truncation, transaction correlation, snapshot/restore, query,
// purge, and the ExcludeEntities integration guard. Fase 2 verification harness.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gostore/internal/audit"
	"gostore/internal/bootstrap"
	"gostore/internal/db"
	"gostore/internal/models"

	goaudit "github.com/gopackx/go-audit"
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

func ptr(s string) *string { return &s }

func main() {
	conn, err := db.Open()
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer conn.Close()

	if err := bootstrap.Fresh(conn.SQL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ctx := context.Background()
	auditor, err := audit.Setup(ctx, conn.Gorm, conn.SQL)
	if err != nil {
		log.Fatalf("audit setup: %v", err)
	}
	check("AutoMigrate creates audit tables", tableExists(conn.SQL, "audit_logs") && tableExists(conn.SQL, "audit_api_logs"),
		"audit_logs + audit_api_logs present")

	gdb := conn.Gorm
	actor := audit.WithUser(ctx, "42", "admin")

	// --- A. Create capture ---
	prod := models.Product{
		UUID: "11111111-1111-1111-1111-111111111111", Name: "Audit Widget", SKU: "AUD-1",
		Details: ptr("first"), Price: 100, Stock: 10, Attributes: ptr(`{"color":"red"}`),
	}
	if err := gdb.WithContext(actor).Create(&prod).Error; err != nil {
		log.Fatalf("create product: %v", err)
	}
	createLogs := query(auditor, ctx, "products", fmt.Sprint(prod.ID), goaudit.ActionCreate)
	check("Create captured", len(createLogs) == 1 && createLogs[0].UserID == "42",
		fmt.Sprintf("%d create log(s), user=%s", len(createLogs), userOf(createLogs)))
	check("Create new_values populated", len(createLogs) == 1 && hasKey(createLogs[0].NewValues, "name"),
		"new_values has all fields")
	// BUG: old_values for a create is persisted as the JSON literal `null`
	// (4 bytes) instead of SQL NULL — jsonMarshal gets a typed-nil map boxed in
	// `any`, so its `v == nil` guard misses it. See FINDINGS.
	check("Create old_values quirk (jsonb 'null' not SQL NULL)", string(firstOld(createLogs)) == "null",
		fmt.Sprintf("old_values=%q (typed-nil marshalled to 'null')", string(firstOld(createLogs))))

	// --- B. Update diff ---
	if err := gdb.WithContext(actor).Model(&prod).Update("price", 150).Error; err != nil {
		log.Fatalf("update: %v", err)
	}
	upd := query(auditor, ctx, "products", fmt.Sprint(prod.ID), goaudit.ActionUpdate)
	check("Update captured with diff", len(upd) == 1 &&
		hasKey(upd[0].NewValues, "price") && !hasKey(upd[0].NewValues, "name"),
		"diff contains only changed field (price)")

	// --- C. ExcludeFields(password) ---
	usr := models.User{UUID: "22222222-2222-2222-2222-222222222222", Name: "Secret User",
		Email: "secret@example.com", Password: "supersecret", Role: "customer", Active: true}
	if err := gdb.WithContext(actor).Create(&usr).Error; err != nil {
		log.Fatalf("create user: %v", err)
	}
	uLogs := query(auditor, ctx, "users", fmt.Sprint(usr.ID), goaudit.ActionCreate)
	check("ExcludeFields drops password", len(uLogs) == 1 && !hasKey(uLogs[0].NewValues, "password") &&
		!strings.Contains(string(uLogs[0].NewValues), "supersecret"),
		"password absent from audit new_values")

	// --- D. Soft delete vs hard delete ---
	soft := models.Product{UUID: "33333333-3333-3333-3333-333333333333", Name: "Soft", SKU: "SOFT-1", Price: 1}
	_ = gdb.WithContext(actor).Create(&soft).Error
	_ = gdb.WithContext(actor).Delete(&soft).Error // soft delete (DeletedAt)
	sd := query(auditor, ctx, "products", fmt.Sprint(soft.ID), goaudit.ActionSoftDelete)
	check("Soft delete -> action soft_delete", len(sd) == 1, fmt.Sprintf("%d soft_delete log(s)", len(sd)))

	// Clean hard delete (a row that was never soft-deleted): works correctly.
	clean := models.Product{UUID: "55555555-5555-5555-5555-555555555555", Name: "CleanHard", SKU: "CH-1", Price: 1}
	_ = gdb.WithContext(actor).Create(&clean).Error
	_ = gdb.WithContext(actor).Unscoped().Delete(&clean).Error
	hd := query(auditor, ctx, "products", fmt.Sprint(clean.ID), goaudit.ActionDelete)
	check("Unscoped (hard) delete -> action delete", len(hd) == 1, fmt.Sprintf("%d delete log(s)", len(hd)))

	_ = gdb.WithContext(actor).Unscoped().Delete(&soft).Error // cleanup

	// BUG (deterministic): an UPDATE/DELETE that uses an explicit string
	// .Where("...") makes the adapter's snapshotRows re-apply the WHERE clause,
	// emitting `SELECT * FROM products WHERE WHERE id = ?` -> 42601 syntax error,
	// which aborts the write. PK-based Model(&x).Update(...) is unaffected.
	bugErr := gdb.WithContext(actor).Model(&models.Product{}).
		Where("id = ?", prod.ID).Update("stock", 7).Error
	check("BUG: .Where(...).Update doubles WHERE in snapshot SELECT", bugErr != nil,
		fmt.Sprintf("err=%v", bugErr))

	// --- E. API logging with redaction (small body) ---
	apiCtx := audit.WithUser(ctx, "42", "admin")
	err = auditor.API().Record(apiCtx, goaudit.APIEntry{
		Service: "payment-gw", Endpoint: "/v1/charge", Method: "POST", StatusCode: 200,
		RequestHeaders: map[string]string{"Authorization": "Bearer SECRET123", "Content-Type": "application/json"},
		RequestBody:    map[string]any{"amount": 1000, "card_number": "4111111111111111", "cvv": "123"},
		ResponseBody:   map[string]any{"status": "ok"},
		DurationMs:     42,
	})
	if err != nil {
		log.Fatalf("api record: %v", err)
	}
	apiLogs, _ := auditor.API().Query(ctx, goaudit.APIFilter{Service: "payment-gw"})
	redactedHeader := len(apiLogs) == 1 && strings.Contains(string(apiLogs[0].RequestHeaders), "REDACTED") &&
		!strings.Contains(string(apiLogs[0].RequestHeaders), "SECRET123")
	redactedBody := len(apiLogs) == 1 && !strings.Contains(string(apiLogs[0].RequestBody), "4111111111111111") &&
		strings.Contains(string(apiLogs[0].RequestBody), "REDACTED")
	check("API redacts Authorization header", redactedHeader, "header redacted, token gone")
	check("API redacts body fields (card_number, cvv)", redactedBody, "card_number + cvv redacted in stored body")

	// --- E2. API truncation (oversized body, separate from redaction) ---
	_ = auditor.API().Record(apiCtx, goaudit.APIEntry{
		Service: "blob-svc", Endpoint: "/upload", Method: "POST", StatusCode: 200,
		RequestBody: map[string]any{"blob": strings.Repeat("x", 4000)}, DurationMs: 5,
	})
	blobLogs, _ := auditor.API().Query(ctx, goaudit.APIFilter{Service: "blob-svc"})
	truncated := len(blobLogs) == 1 && strings.Contains(string(blobLogs[0].RequestBody), "_truncated")
	check("API truncates oversized body", truncated, "body > MaxBodySize -> _truncated envelope")

	// --- F. Transaction correlation ---
	txID := goaudit.NewTransactionID()
	txCtx := goaudit.WithTransactionID(audit.WithUser(ctx, "42", "admin"), txID)
	if err := gdb.WithContext(txCtx).Model(&prod).Update("stock", 99).Error; err != nil {
		log.Fatalf("tx update: %v", err)
	}
	_ = auditor.API().Record(txCtx, goaudit.APIEntry{
		Service: "shipping", Endpoint: "/quote", Method: "POST", StatusCode: 200,
		ResponseBody: map[string]any{"eta": "2d"}, DurationMs: 10,
	})
	txLog, err := auditor.QueryByTransaction(ctx, txID)
	if err != nil {
		log.Fatalf("QueryByTransaction: %v", err)
	}
	check("txID correlates data-change + API-call", len(txLog.DataLogs) == 1 && len(txLog.APILogs) == 1,
		fmt.Sprintf("data=%d api=%d under one txID", len(txLog.DataLogs), len(txLog.APILogs)))

	// --- G. Snapshot & Restore (time travel) ---
	tt := models.Product{UUID: "44444444-4444-4444-4444-444444444444", Name: "TimeTravel", SKU: "TT-1", Price: 10}
	_ = gdb.WithContext(actor).Create(&tt).Error
	time.Sleep(15 * time.Millisecond)
	_ = gdb.WithContext(actor).Model(&tt).Update("price", 20).Error
	midpoint := time.Now().UTC()
	time.Sleep(15 * time.Millisecond)
	_ = gdb.WithContext(actor).Model(&tt).Update("price", 30).Error
	_ = gdb.WithContext(actor).Model(&tt).Update("price", 40).Error

	snap, err := auditor.Snapshot(ctx, "products", fmt.Sprint(tt.ID), midpoint)
	if err != nil {
		log.Fatalf("snapshot: %v", err)
	}
	check("Snapshot reconstructs past state", snap != nil && numEq(snap["price"], 20),
		fmt.Sprintf("price@midpoint=%v (expected 20)", snap["price"]))

	res, err := auditor.Restore(ctx, "products", fmt.Sprint(tt.ID), midpoint)
	if err != nil {
		log.Fatalf("restore: %v", err)
	}
	if res.Values != nil {
		// Apply via PK-based Model(&tt) to avoid the .Where(...) snapshot bug.
		_ = gdb.WithContext(ctx).Model(&tt).Updates(map[string]any{"price": res.Values["price"]}).Error
	}
	restoreLogs := query(auditor, ctx, "products", fmt.Sprint(tt.ID), goaudit.ActionRestore)
	check("Restore records action=restore", len(restoreLogs) == 1 && numEq(res.Values["price"], 20),
		fmt.Sprintf("restore entry written, target price=%v", res.Values["price"]))

	// --- H. Query + Purge ---
	allProducts, _ := auditor.Query(ctx, goaudit.DataFilter{EntityType: "products", Limit: 500})
	check("Query by EntityType", len(allProducts) > 0, fmt.Sprintf("%d product audit rows", len(allProducts)))
	pr, err := auditor.Purge(ctx, time.Now().UTC().Add(time.Hour)) // purge everything (cutoff in future)
	if err != nil {
		log.Fatalf("purge: %v", err)
	}
	after, _ := auditor.Query(ctx, goaudit.DataFilter{EntityType: "products", Limit: 500})
	check("Purge deletes old rows", pr.DataLogs > 0 && len(after) == 0,
		fmt.Sprintf("purged data=%d api=%d, remaining=%d", pr.DataLogs, pr.APILogs, len(after)))

	// --- I. ExcludeEntities integration guard (notifications not audited) ---
	if _, err := conn.SQL.Exec(`CREATE TABLE IF NOT EXISTS notifications (
		id BIGSERIAL PRIMARY KEY, type VARCHAR(255), data JSONB, created_at TIMESTAMPTZ DEFAULT now())`); err != nil {
		log.Fatalf("create notifications: %v", err)
	}
	notif := notifRow{Type: "Welcome", Data: ptr(`{"msg":"hi"}`)}
	if err := gdb.WithContext(actor).Create(&notif).Error; err != nil {
		log.Fatalf("create notif: %v", err)
	}
	notifAudit, _ := auditor.Query(ctx, goaudit.DataFilter{EntityType: "notifications", Limit: 10})
	check("ExcludeEntities keeps notifications out of audit", len(notifAudit) == 0,
		fmt.Sprintf("%d audit rows for notifications (expected 0)", len(notifAudit)))

	fmt.Println("\n================ Fase 2 coverage ================")
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

type notifRow struct {
	ID   uint64 `gorm:"primaryKey"`
	Type string
	Data *string `gorm:"type:jsonb"`
}

func (notifRow) TableName() string { return "notifications" }

func query(a goaudit.Auditor, ctx context.Context, entity, id, action string) []goaudit.AuditLog {
	logs, err := a.Query(ctx, goaudit.DataFilter{EntityType: entity, EntityID: id, Action: action, Limit: 50})
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	return logs
}

func firstNew(logs []goaudit.AuditLog) json.RawMessage {
	if len(logs) == 0 {
		return nil
	}
	return logs[0].NewValues
}

func firstOld(logs []goaudit.AuditLog) json.RawMessage {
	if len(logs) == 0 {
		return nil
	}
	return logs[0].OldValues
}

func userOf(logs []goaudit.AuditLog) string {
	if len(logs) == 0 {
		return ""
	}
	return logs[0].UserID
}

func hasKey(raw json.RawMessage, key string) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func numEq(v any, want float64) bool {
	switch n := v.(type) {
	case float64:
		return n == want
	case int:
		return float64(n) == want
	case json.Number:
		f, _ := n.Float64()
		return f == want
	case string:
		return n == fmt.Sprintf("%g", want) || n == fmt.Sprintf("%.2f", want)
	default:
		return false
	}
}

func tableExists(sqlDB *sql.DB, name string) bool {
	var n int
	_ = sqlDB.QueryRow(
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, name,
	).Scan(&n)
	return n > 0
}
