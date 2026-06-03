# FINDINGS — Dogfooding the `gopackx` stack in GoStore

> Living document. Setiap error / DX ribet / README≠behavior / friksi antar-package dicatat di sini.
> Severity: `BUG` / `MISSING` / `DX` / `DOCS` / `INTEGRATION`.

Entry format:
```
### [SEVERITY] Judul — [PACKAGE]
- Fitur / titik integrasi:
- Ekspektasi:
- Aktual:
- Repro (snippet minimal):
- Saran:
```

---

## Environment / Setup

### [DX] Tidak ada Docker / Postgres lokal — pakai embedded-postgres — [env]
- Fitur / titik integrasi: bootstrap DB (Fase 0).
- Ekspektasi: `docker compose up -d` Postgres 16 (sesuai plan).
- Aktual: mesin dev tidak punya Docker maupun Postgres lokal (port 5432 closed, tidak ada service `postgres*`, `psql` tidak ada).
- Repro: `docker --version` → not found; TcpClient ke localhost:5432 → CLOSED.
- Solusi yang dipakai: `github.com/fergusstrange/embedded-postgres` (download binary Postgres asli zonky, jalan tanpa Docker). Dialect tetap **PostgreSQL** asli, jadi semua uji grammar Postgres tetap valid. Override via `DATABASE_URL` kalau ada Postgres beneran.
- Catatan: ini bukan bug package — murni kondisi environment. Dicatat untuk reproducibility.

### [DOCS] go-audit GORM adapter tidak punya tag semver — [go-audit]
- Fitur / titik integrasi: `go get github.com/gopackx/go-audit/adapters/gorm`.
- Ekspektasi: README nulis `go get github.com/gopackx/go-audit/adapters/gorm` (kesannya ada rilis tag).
- Aktual: submodule adapter tidak punya tag; `go get ...@latest` resolve ke pseudo-version `v0.0.0-20260417032849-9aa53af2642b`. `@v1.0.0` gagal (`module ... found, but does not contain package .../adapters/gorm` — adapter adalah modul terpisah, bukan bagian dari go-audit@v1.0.0).
- Saran: tag adapter submodule (mis. `adapters/gorm/v1.0.0`) biar bisa di-pin reproducible, atau dokumentasikan bahwa adapter dipasang via pseudo-version.

---

## go-migration

> Verdict Fase 1: **solid.** 23/23 coverage checks PASS (lihat `cmd/migrate-demo`).
> Up/Status/Rollback(batch & step)/Reset/Refresh/Fresh, hooks, seeder dependency
> order, circular detection, factory states, dan DisableTransaction opt-out semua
> jalan benar di Postgres asli. Temuan di bawah mayoritas DOCS/DX, bukan BUG.

### [DOCS] README foreign-key memakai `.OnDelete(...)` yang tidak ada — [go-migration]
- Fitur: schema builder foreign key.
- Ekspektasi: README Quick Start: `bp.Foreign("author_id").References("id").On("users").OnDelete("CASCADE")`.
- Aktual: tidak compile. `OnDelete` adalah **field** pada `ForeignKeyDefinition`, bukan method. Method yang benar: `.OnDeleteAction("CASCADE")` / `.OnUpdateAction(...)`.
- Repro: `pkg/schema/foreign_key.go` hanya punya `OnDeleteAction`/`OnUpdateAction`.
- Saran: perbaiki README, atau tambahkan alias method `OnDelete(...)`/`OnUpdate(...)` biar contoh README valid.

### [DX] `make:seeder` tidak meng-`AutoRegister` (beda dgn `make:migration`) — [go-migration]
- Fitur: CLI scaffolding + auto-discovery (`db:seed`).
- Ekspektasi: konsisten — kalau migration hasil generate auto-register lewat `init()`, seeder juga.
- Aktual: `make:migration` menulis `func init(){ migrator.AutoRegister(...) }`, tapi `make:seeder` **tidak** menulis registrasi apa pun. `pkg/migrator/run.go` melakukan auto-discovery seeder via `seeder.GetAutoRegistered()`, jadi seeder hasil generate **tidak akan terpanggil** `db:seed` tanpa wiring manual.
- Repro: `migrator make:seeder InvoiceSeeder` → file tanpa `init()`/AutoRegister.
- Saran: emit `init(){ seeder.AutoRegister("InvoiceSeeder", &InvoiceSeeder{}) }` di scaffold seeder (dan factory) seperti migration.

### [DX] Nama package di file hasil generate di-hardcode — [go-migration]
- Fitur: CLI scaffolding.
- Aktual: `make:migration` selalu `package migrations`, `make:seeder` selalu `package seeders`, walau `migration_dir`/`seeder_dir` menunjuk folder lain (mis. `internal/seed` → file ber-`package seeders`, mismatch dgn folder).
- Saran: turunkan nama package dari basename folder tujuan, atau sediakan flag `--package`.

### [DOCS] Format config YAML didokumentasikan tapi CLI bilang deprecated — [go-migration]
- Fitur: konfigurasi CLI.
- Ekspektasi: README section "Configuration" menampilkan contoh **YAML** sebagai format utama ("Supports YAML, JSON, or environment variables").
- Aktual: tiap perintah CLI dgn `--config config.yaml` mencetak `WARNING: YAML configuration format is deprecated and will be removed in a future version. Please migrate to JSON format.`
- Saran: selaraskan — entah un-deprecate YAML, atau update README untuk mempromosikan JSON & menandai YAML deprecated.

### [DX] Perilaku Postgres yang benar tapi tak terdokumentasi (silent) — [go-migration]
- `JSON()` selalu → `JSONB` di Postgres (tidak ada opsi untuk tipe `json` polos). Bagus untuk indexing, tapi tak ada cara opt-out; sebut di docs.
- `Unsigned()` **diabaikan diam-diam** di Postgres (tidak error, tidak warning). `BigInteger("x").Unsigned()` = `BIGINT`. Sesuai realita (PG tak punya unsigned) tapi sebaiknya didokumentasikan / opsional warning.
- `ID()` = `BIGSERIAL PRIMARY KEY` (set Unsigned+AutoIncrement+Primary). `Float()` → `DOUBLE PRECISION`. `Timestamp` → `TIMESTAMPTZ`. Semua wajar; catat di docs tipe-mapping per dialect.
- Positif: regular `Index` di-compile jadi statement `CREATE INDEX` terpisah yang digabung satu string ber-`;` dan dieksekusi satu `Exec` — **jalan** di driver pgx (gorm postgres). Tidak ada masalah multi-statement.

## go-audit

> Verdict Fase 2: **kuat, tapi ada 1 BUG tajam.** 18/18 coverage checks PASS
> (lihat `cmd/audit-demo`): auto-capture CUD + diff, ExcludeFields, soft/hard
> delete, API redaction header+body + truncation, txID correlation, snapshot/
> restore time-travel, query, purge, ExcludeEntities. Bug `WHERE WHERE` di bawah
> wajib diperbaiki sebelum dipromosiin.

### [BUG] GORM adapter: `.Where("...").Update/Delete` menghasilkan `WHERE WHERE` (42601) — [go-audit/adapters/gorm]
- Fitur: snapshot old_values sebelum UPDATE/DELETE (`callbacks.go:snapshotRows`, line ~142).
- Ekspektasi: `db.Model(&X{}).Where("id = ?", id).Update(...)` ke-audit normal.
- Aktual: snapshot SELECT yang di-generate **double WHERE**:
  `SELECT * FROM "products" WHERE WHERE id = 1 AND "products"."deleted_at" IS NULL`
  → `ERROR: syntax error at or near "WHERE" (SQLSTATE 42601)`. Di mode default
  `ErrorFailLoud` error ini di-`AddError` ke `*gorm.DB`, sehingga **membatalkan
  tulisan user** (UPDATE/DELETE tidak jalan). Pola `Model(&x).Update(...)`
  berbasis primary key TIDAK kena (WHERE diturunkan dari PK).
- Root cause: `snapshotRows` mengambil `db.Statement.Clauses["WHERE"]` lalu
  memasangnya ulang via `q.Clauses(c)` pada query baru. Untuk statement yang
  WHERE-nya dibangun dari kondisi string `.Where("...")`, klausa itu sudah
  membawa keyword `WHERE`, jadi tergandakan.
- Repro (deterministik, ada di `cmd/audit-demo`):
  ```go
  gdb.WithContext(ctx).Model(&models.Product{}).Where("id = ?", id).Update("stock", 7)
  // err: go-audit: snapshot before update: ERROR: syntax error at or near "WHERE"
  ```
- Dampak: tinggi — `Model().Where(string).Update/Delete` adalah pola GORM yang
  sangat umum. Semua write seperti ini gagal saat plugin aktif.
- Saran: jangan re-inject klausa WHERE mentah; bangun ulang kondisi via
  `q.Where(db.Statement.Clauses["WHERE"].Expression)` atau salin `conds`-nya,
  bukan `Clauses(c)`. Tambahkan test untuk update/delete berbasis string-where.

### [BUG] `old_values` pada CREATE tersimpan sebagai literal JSON `null`, bukan SQL NULL — [go-audit]
- Fitur: `RecordDataChange` untuk action create.
- Ekspektasi: kolom `old_values` = SQL `NULL` saat create (tidak ada state lama).
- Aktual: tersimpan sebagai jsonb literal `null` (4 byte). Penyebab: `diffResult`
  mengembalikan `oldOut` bertipe `map[string]any` **nil**, lalu di-pass ke
  `jsonMarshal(v any)`. Guard `if v == nil` meleset karena nil-map yang di-box ke
  `any` ≠ `nil` → `json.Marshal(nilMap)` = `null`.
- Repro: create apa pun lewat GORM → `SELECT old_values` = `null` (string), bukan NULL.
- Dampak: kosmetik tapi membingungkan konsumen yang membedakan NULL vs `null`.
- Saran: di `jsonMarshal` cek juga nil via reflect (`reflect.ValueOf(v).IsNil()` untuk map/slice/ptr), atau di `RecordDataChange` lewatkan `nil` eksplisit saat map kosong.

### [DX/DOCS] `RedactBodyFields` tidak meredaksi body bertipe struct — [go-audit]
- Fitur: redaksi body API (`redactBody`/`redactWalk`).
- Ekspektasi: README API logging memakai `RequestBody: req` di mana `req` adalah **struct**.
- Aktual: `redactWalk` hanya menangani `map[string]any` & `[]any`; struct jatuh
  ke `default` dan dikembalikan apa adanya → **tidak ter-redaksi**. Jadi contoh
  README (struct) menyimpan field sensitif tanpa redaksi. Yang ter-redaksi hanya
  body yang sudah berupa `map[string]any` (terbukti PASS di demo saat pakai map).
- Saran: marshal→unmarshal ke `map[string]any` sebelum walk, atau reflect ke
  struct; minimal dokumentasikan bahwa body harus map/slice agar redaksi jalan.

### [DOCS] `ExcludeEntities` ada tapi tidak didokumentasikan — [go-audit]
- Fitur: exclude **tabel** dari audit (`DataAuditConfig.ExcludeEntities`).
- Konteks: plan menduga exclude-table mungkin `MISSING` (README cuma menampilkan
  `ExcludeFields`). Faktanya `ExcludeEntities []string` **ada dan bekerja** —
  GoStore memakainya untuk menjaga tabel `notifications` (+ tabel audit/infra)
  keluar dari trail (demo: 0 audit row untuk `notifications`). Ini menyelesaikan
  friksi "audit menelan notifikasi".
- Saran: dokumentasikan `ExcludeEntities` di README (sangat berguna untuk integrasi).

### [DX] Catatan kecil
- `APIEntry` tidak punya `ResponseHeaders` (hanya `RequestHeaders`), walau bagian
  "Schema" README menyebut "request/response headers". Konsisten kan dokumen vs tipe.
- Default mode `ErrorFailLoud` mengikat keberhasilan write app ke ketersediaan
  audit store — wajar untuk compliance, tapi kombinasi dgn bug `WHERE WHERE` di
  atas membuat write gagal total. `ErrorFailSilent` tersedia sebagai mitigasi.

## go-notification

_(diisi Fase 5)_

## open-swag-go

_(diisi Fase 4 & 7)_

## Integrasi

_(table ownership, audit-menelan-notif, txID, versi Go/go.mod — diisi sepanjang fase)_

## Ringkasan — Top 5 sebelum dipromosiin

_(diisi di akhir)_
