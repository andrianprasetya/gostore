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

_(diisi Fase 2)_

## go-notification

_(diisi Fase 5)_

## open-swag-go

_(diisi Fase 4 & 7)_

## Integrasi

_(table ownership, audit-menelan-notif, txID, versi Go/go.mod — diisi sepanjang fase)_

## Ringkasan — Top 5 sebelum dipromosiin

_(diisi di akhir)_
