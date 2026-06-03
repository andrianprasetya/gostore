# Plan: GoStore — Dogfood & Stress-Test the `gopackx` Stack

> **Tujuan (3 sekaligus):**
> 1. **Dogfood 4 package** dalam SATU app realistis → cari fitur kurang / bug / friksi integrasi.
> 2. Stress-test titik integrasi antar-package (di sinilah bug paling berharga muncul).
> 3. Hasilkan bahan **post LinkedIn**: cerita "Laravel-style DX untuk Go" (migration + notification + audit + API docs).
>
> **Aturan main agent:** tiap ada error / DX ribet / README gak cocok behavior / friksi antar-package → **catat di `FINDINGS.md`** (Fase 9), jangan diam-diam workaround. Itu deliverable terpenting.

## Package yang diuji

| Package | Peran di GoStore | Butuh |
| --- | --- | --- |
| `open-swag-go` | API documentation (UI Scalar, try-it, version-diff) | framework (Gin) |
| `go-migration` | schema + seeder (DDL, factory, CLI) | `database/sql` |
| `go-audit` | audit trail data-change + API-call logging | ORM (GORM) + DB |
| `go-notification` | notifikasi multi-channel (in-app + test channel) | Go 1.22+ |

## Keputusan teknis

- **DB:** **PostgreSQL** (relatable ke stack company). Jalankan lokal via Docker (`docker-compose.yml`) biar reproducible — gak nyemarin mesin dev. Driver: `pgx` (atau `lib/pq`).
- **ORM:** GORM (jalur paling lurus buat go-audit; cocok sama README-nya).
- **Bridge:** GORM buka koneksi ke Postgres, ambil `*sql.DB` via `gormDB.DB()`, **pakai `*sql.DB` yang sama** buat `go-migration` (grammar `PostgresGrammar`). Satu DB, satu pool.
- **Framework:** Gin (utama). Chi dipakai hanya buat uji adapter kedua `open-swag-go`.
- **Notifikasi:** pakai **database channel** (in-app, simpan ke Postgres, `DialectPostgreSQL`) + **1 custom test channel** (cuma nge-log) supaya GAK perlu kredensial SMTP/Twilio asli. Driver asli (Mailtrap/SMTP) opsional via env.
- **Config/DSN:** lewat env (`DATABASE_URL=postgres://gostore:gostore@localhost:5432/gostore?sslmode=disable`) atau `config.yaml` — jangan hardcode kredensial.

## ⚠️ Titik integrasi paling rawan (perhatikan dari awal)

1. **Kepemilikan tabel / siapa yang migrate.** Tiga package bikin tabel sendiri:
   - `go-migration` → tabel app (`users`, `products`, `orders`) + tabel tracking `migrations`
   - `go-audit` → `audit_logs`, `audit_api_logs` (lewat `AutoMigrate`)
   - `go-notification` → `notifications` (lewat `migrate.Up`)
   Tentukan urutan & ownership yang jelas. Catat kalau ada bentrok nama/urutan.
2. **Audit "menelan" tulisan notifikasi.** Tulisan ke tabel `notifications` lewat GORM bisa ikut ke-audit → noise. Cek apakah go-audit punya cara exclude **tabel** (README cuma nunjukin `ExcludeFields`, bukan exclude table). Kalau gak ada → kandidat `MISSING`.
3. **Kompatibilitas versi Go / go.mod** keempat package + GORM + driver Postgres (`pgx`/`lib/pq`). Cek konflik di Fase 0.
4. **Propagasi `transaction_id`.** Middleware HTTP set txID di `ctx` → harus kebawa ke audit data-change, audit API-call, dan notifikasi dalam satu request.

## Struktur project (target)

```
gostore/
├── go.mod
├── docker-compose.yml          # Postgres 16 lokal (db: gostore, user/pass: gostore)
├── .env.example                # DATABASE_URL, dll
├── cmd/
│   ├── server/main.go          # Gin (utama): migrate→seed→serve
│   ├── server-chi/main.go      # uji adapter open-swag-go kedua
│   ├── migrator/main.go        # CLI go-migration (atau pakai CLI bawaan)
│   └── gen-spec/main.go        # export openapi.json
├── internal/
│   ├── migrations/             # struct migration (Up/Down) + seeders + factories
│   ├── models/                 # GORM models (users, products, orders, order_items)
│   ├── dto/                    # request/response struct + edge cases
│   ├── handlers/               # handler + co-located *Doc (pola decorator)
│   ├── audit/                  # setup auditor + middleware txID
│   ├── notify/                 # notifikasi (Welcome, OrderPlaced) + custom test channel
│   └── docs/                   # setup open-swag-go
├── spec/{openapi-v1.json,openapi-v2.json}
├── frontend-types/
├── linkedin-assets/
├── FINDINGS.md                 # ⭐ deliverable utama
└── README.md
```

---

## Fase 0 — Setup & cek kompatibilitas

1. Bikin `docker-compose.yml` (Postgres 16) + `.env.example`; `docker compose up -d`; tunggu healthy.
2. `go mod init gostore`; `go version` (catat; go-notification butuh 1.22+).
3. `go get` keempat package + adapter go-audit GORM + GORM + driver Postgres GORM (`gorm.io/driver/postgres`) + `pgx`.
4. Resolve `go.mod`. **Catat konflik versi / require yang aneh** di FINDINGS.
5. Buka koneksi GORM→Postgres pakai `DATABASE_URL`, ambil `*sql.DB`. Smoke test: `db.Ping()`.

## Fase 1 — `go-migration` (schema, seeder, factory, CLI)

**Migrations** (struct-based, `Up`/`Down`) buat: `users`, `products`, `orders`, `order_items`. Sengaja pakai banyak tipe & modifier:
- `ID`, `String`, `Text().Nullable()`, `BigInteger().Unsigned()`, `Decimal(10,2)`, `Boolean().Default()`, `UUID`, `JSON().Nullable()`, `Timestamps()`, `SoftDeletes()`
- `Index`, `UniqueIndex` gabungan, `Foreign(...).References(...).On(...).OnDelete("CASCADE")`
- **Edge Postgres:** `Alter` + `DropColumn` + `Rename`; cek `JSON` di-map ke `json` vs `jsonb` (penting di Postgres); `UUID` pakai tipe native `uuid` atau `varchar`?; `Unsigned()` gak ada di Postgres → lihat apakah package abaikan/error. `HasTable`/`HasColumn`.
- 1 migration dgn **`DisableTransaction()`** — di Postgres DDL transaksional, jadi default-nya migration gagal harus auto-rollback bersih; verifikasi opt-out beneran beda perilaku.
- pasang **Before/After hooks** (log durasi)

**Migrator API:** uji `Up`, `Status`, `Rollback(0)` (batch), `Rollback(2)` (step), `Reset`, `Refresh`, `Fresh`. Verifikasi batch tracking bener.

**Seeder + factory:**
- `UserSeeder`, `ProductSeeder`, `OrderSeeder(DependsOn: User, Product)` → uji dependency order
- **Sengaja bikin circular dependency** (A→B→A) → cek deteksi circular jalan
- Factory + faker: `Make()`, `MakeMany(20)`, `State("admin", ...)` + `WithState(...)`

**CLI:** build `cmd/migrator` (atau CLI bawaan), uji `make:migration --create`, `make:migration --table`, `make:seeder`, `migrate`, `migrate:rollback --step`, `migrate:status`, `db:seed --class`.

→ Coverage checklist + temuan ke FINDINGS (kategori per fitur).

## Fase 2 — `go-audit` (audit trail otomatis)

1. GORM models map ke tabel hasil migration.
2. `audit.New(sqlDB, Config{ Dialect: PostgreSQL, UserFunc: ..., DataAudit: {ExcludeFields: ["password"]}, APIAudit: {RedactHeaders, RedactBodyFields, MaxBodySize} })`; `AutoMigrate(ctx)`; `gormDB.Use(auditgorm.Plugin(auditor))`.
3. Uji auto-capture:
   - Create/Update/Delete product lewat GORM → cek `audit_logs` isi `old_values`/`new_values` + diff bener
   - `ExcludeFields` beneran buang `password`
   - **Soft delete:** model dgn `gorm.DeletedAt` → `db.Delete` jadi `action: soft_delete`; `Unscoped().Delete` jadi `delete`
4. **API call logging:** simulasikan panggil "payment gateway" (mock HTTP) → `auditor.API().Record(...)` dgn redaction header `Authorization` + truncation body.
5. **Transaction correlation:** `txID := audit.NewTransactionID(); ctx = audit.WithTransactionID(ctx, txID)` → 1 data-change + 1 API-call share txID; `QueryByTransaction(ctx, txID)` balikin dua-duanya.
6. **Snapshot & Restore** (demo "time travel" — kuat buat LinkedIn): ubah harga product beberapa kali, `Snapshot(ctx,"products",id,kemarin)`, lalu `Restore(...)` → cek entry `action: restore`.
7. `Query` (DataFilter + APIFilter) dan `Purge(before)`.
8. **Friksi integrasi (PENTING):** tulisan ke tabel `notifications` (Fase 5) lewat GORM bakal ikut ke-audit? Cari cara exclude **tabel**. Kalau gak ada → FINDINGS `MISSING`.

## Fase 3 — Handler domain (Gin)

CRUD beneran (lewat GORM) yang otomatis ke-audit:
- `POST /auth/register`, `POST /auth/login`
- `GET /products`, `GET /products/{id}`, `POST /products` (bearer), `POST /products/{id}/image` (file upload)
- `POST /orders` (nested `[]OrderItem`), `GET /orders/{id}`, `GET /admin/orders` (apiKey)
- **Middleware:** generate `transaction_id`, taruh di `ctx` (dipakai audit + notif).

## Fase 4 — `open-swag-go` (dokumentasi API)

Co-located `*Doc` di tiap handler + `AddAll(...)`. Hajar semua fitur (ringkas dari rencana awal):
- Auth scheme: Bearer / APIKey / Basic / Cookie (keempatnya)
- Param: path / query / header / required-query
- Body: `Body` / `BodyWithDesc` / `FormBody` (upload)
- Struct tag mentok + **edge type**: pointer, `time.Time`, slice-of-struct, nested, **recursive** (`Category.Children`), `map`, no-json-tag, enum, `format:"uuid"`
- Response 200/201/400/401/404/500 + error struct
- UI: theme `purple`/`dark`/`light` + `DarkMode` + sidebar → screenshot tiap theme
- Verifikasi try-it console balikin response asli (karena handler beneran jalan), auth playground inject token, code-snippet 5 bahasa, `/docs/openapi.json` valid.
- **Adapter kedua:** ulang mount via Chi (`cmd/server-chi`), bandingkan output identik sama Gin.

## Fase 5 — `go-notification` (multi-channel)

1. `notifiable`: `User` implement `GetID()` + `RouteNotificationFor(ch)`.
2. Notifikasi: `Welcome` (saat register) dan `OrderPlaced` (saat order), masing-masing `Via()` → `["database", "test"]`.
3. Channel:
   - **database channel** → simpan in-app ke Postgres; uji `Unread(ctx,user,20)`, `MarkAllAsRead`; `migrate.Up(ctx, db, DialectPostgreSQL, "notifications")`
   - **custom test channel** (`Name()="test"`, `Send` cuma log) → bukti pluggability tanpa kredensial asli
4. Async/retry/ratelimit: `Config{WorkerPool:10, MaxRetries:3, RetryDelay, OnError}`; `SetRateLimit("test",100,time.Second,20)`.
5. Override saat kirim: `notification.Sync()` (blok sampai terkirim) dan `notification.Via("database")`.
6. Uji `OnError` kepanggil (paksa 1 channel gagal), dan `Close()` nge-drain worker pool.
7. (Opsional) wire SMTP/Mailtrap via env kalau user mau bukti email beneran.

→ Catat: thread-safety, urutan async, apakah `Sync()` beneran blocking, perilaku saat pool penuh (backpressure).

## Fase 6 — Skenario end-to-end (⭐ showcase utama)

Satu request, empat package jalan bareng:
1. `POST /orders` → middleware bikin `txID`.
2. GORM simpan order → **go-audit** catat data-change (txID).
3. Panggil mock payment API → **go-audit** catat API-call (txID, header ter-redact).
4. Kirim **go-notification** `OrderPlaced` (database + test channel).
5. `GET /audit/transactions/{txID}` → tampilkan data-change + API-call yang terkorelasi.
6. `GET /notifications` → in-app notifications user.

Verifikasi: satu `txID` benar-benar mengikat data-change + API-call. Ini inti screenshot/demo LinkedIn.

## Fase 7 — Version diff (`open-swag-go`)

Generate `openapi-v1.json`, bikin breaking change (mis. `Product.Price int` → `{amount,currency}`), generate `openapi-v2.json`, jalankan `pkg/versioning` → verifikasi breaking change kedeteksi + pesan migrasi masuk akal.

## Fase 8 — TS type-gen (story full-stack)

Server jalan → `npx openapi-typescript http://localhost:8080/docs/openapi.json -o ./frontend-types/api.d.ts`. Cek nested/array/enum/`uuid` kebawa bener. Catat mismatch.

## Fase 9 — `FINDINGS.md` (deliverable utama)

Buat dari awal, isi terus. Per entry:
```
### [SEVERITY] Judul — [PACKAGE]
- Fitur / titik integrasi:
- Ekspektasi:
- Aktual:
- Repro (snippet minimal):
- Saran:
```
`SEVERITY`: `BUG` / `MISSING` / `DX` / `DOCS` / `INTEGRATION`.

Section wajib:
- per-package (open-swag-go / go-migration / go-audit / go-notification)
- **Integrasi** (table ownership, audit-menelan-notif, txID, versi Go/go.mod)
- Ringkasan: **Top 5** hal yang paling worth diperbaiki sebelum dipromosiin.

## Fase 10 — Bahan LinkedIn (`linkedin-assets/`)

1. Screenshot `/docs/` (dark, sidebar, banyak endpoint).
2. Screenshot try-it balikin response asli.
3. **Demo korelasi txID** (data-change + API-call satu transaksi) — paling kuat.
4. Demo **snapshot/restore** (time-travel) go-audit.
5. Cuplikan migration struct + seeder/factory (DX ala Laravel).
6. Cuplikan satu notifikasi fan-out ke banyak channel.
7. Output version-diff (breaking change kedeteksi).
8. Angka kalau ada (mis. "0 baris YAML OpenAPI ditulis tangan", "X tabel ter-migrate via struct").

> Caption LinkedIn TIDAK ditulis di sini — disusun terpisah setelah tahu hasil & temuan nyata.

---

## Deliverables

1. `gostore` jalan: PostgreSQL (Docker) + GORM + Gin, migrate→seed→serve.
2. Bukti tiap package kepakai (audit logs, notifications, docs UI, migration status).
3. Skenario end-to-end txML correlation jalan.
4. `spec/openapi-v{1,2}.json` + version-diff; `frontend-types/api.d.ts`.
5. **`FINDINGS.md`** lengkap (paling penting).
6. `linkedin-assets/`.

## Catatan buat agent

- Pakai **PostgreSQL** via `docker compose up -d` (lokal). Kalau Docker gak ada, fallback ke Postgres lokal/remote via `DATABASE_URL`. Semua dialect di-set `PostgreSQL`/`PostgresGrammar`.
- **Jangan** pakai kredensial provider asli; cukup database channel + custom test channel. Kredensial DB lewat env, jangan hardcode.
- Kalau signature di README beda dgn package terpasang → ikut package asli, catat di FINDINGS (`DOCS`).
- Commit per fase. **Jangan push ke remote tanpa konfirmasi pemilik repo.**
- Kalau satu package bikin agent stuck > wajar, lanjut package lain dulu dan catat blocker-nya — jangan macet di satu titik.
