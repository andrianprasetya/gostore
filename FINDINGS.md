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

> Verdict Fase 4: **engine schema bagus, tapi README menyesatkan + 2 BUG.**
> Spec valid OpenAPI 3.1.0, edge-type mapping benar, adapter Gin == Chi (byte-identik),
> UI Scalar render, try-it manggil handler asli. Tapi seluruh contoh README pakai
> API yang tidak ada di v1.1.1, multipart tak terdokumentasi, dan tipe rekursif
> bikin crash.

### [DOCS] README mendokumentasikan API yang TIDAK ADA di v1.1.1 — [open-swag-go]
- Fitur: hampir seluruh contoh README (Quick Start, Co-located, Request Body, Parameters, Auth, Config).
- Ekspektasi (README): helper `openswag.Body(T{})`, `BodyWithDesc(...)`, `FormBody(...)`,
  `Response("desc", T{})`, `Responses{...}`, `PathParam/QueryParam/HeaderParam/RequiredQueryParam(...)`,
  `BearerAuth/APIKeyAuth/BasicAuth/CookieAuth(...)`, dan `Config.Auth = AuthConfig{Schemes: [...]}`.
- Aktual (v1.1.1): **tidak satupun fungsi/field itu ada** (dicek: `grep` di seluruh package nihil).
  API sebenarnya berbasis struct-literal:
  ```go
  RequestBody: &openswag.RequestBody{Schema: T{}, Required: true, ContentType: "application/json"},
  Responses:   map[int]openswag.Response{201: {Description: "Created", Schema: T{}}},
  Parameters:  []openswag.Parameter{{Name: "id", In: "path", Required: true}},
  Security:    []string{openswag.SecurityBearerAuth}, // konstanta string predefined
  ```
  Tidak ada `Config.Auth`; security scheme otomatis ditambah dari `Endpoint.Security`.
- Dampak: tinggi — hal pertama yang dicopy user (Quick Start) **tidak compile**.
- Saran: sinkronkan README dengan API v1.1.1, atau sediakan helper yang dijanjikan README.

### [BUG] `RequestBody.ContentType` diabaikan — multipart/form-data tak bisa didokumentasikan — [open-swag-go]
- Fitur: file upload (`POST /products/{id}/image`, ContentType `multipart/form-data`).
- Ekspektasi: requestBody di spec ber-content `multipart/form-data`.
- Aktual: `buildOperation` menghitung `contentType` dari `ep.RequestBody.ContentType`
  tapi **tidak pernah memakainya** — selalu `spec.NewRequestBody(...).WithJSONContent(s)`.
  Hasil spec selalu `application/json`. (`spec/openapi-v1.json` → `/products/{id}/image`
  content-types = `["application/json"]`).
- Dampak: endpoint upload terdokumentasi salah; konsep `FormBody` (README) tak ada efek.
- Saran: pakai `ContentType` saat membangun requestBody (`WithContent(contentType, s)`).

### [BUG] Tipe rekursif/self-referential → stack overflow saat generate spec — [open-swag-go]
- Fitur: `schema.fromReflectType` (rekursi struct field).
- Ekspektasi: tipe seperti `Category{ Children []Category }` (tree/menu/komentar berjenjang) menghasilkan schema (idealnya pakai `$ref`).
- Aktual: tidak ada visited-set/guard kedalaman → rekursi tak berujung →
  `fatal error: stack overflow` (tidak bisa di-recover). Repro: `cmd/recursion-test`
  → `runtime: goroutine stack exceeds 1000000000-byte limit`.
- Dampak: tinggi — model self-referential apa pun meng-crash generate spec/boot server.
- Saran: lacak tipe yang sudah dikunjungi dan emit `$ref` ke `components.schemas`
  (sekaligus memperkecil spec), atau minimal batasi kedalaman + error rapi.

### [DX/DOCS] Tidak ada Cookie auth walau README menampilkan `CookieAuth` — [open-swag-go]
- Predefined hanya: `SecurityBearerAuth`(http bearer), `SecurityBasicAuth`,
  `SecurityApiKey`(header X-API-Key), `SecurityApiKeyQuery`(?api_key=), `SecurityOAuth2`.
- Nama scheme custom apa pun **diam-diam** jadi http-bearer (cabang `default`). Cookie auth (README) tak ada.
- Saran: tambah cookie scheme, atau hapus dari README; dokumentasikan perilaku default custom-scheme.

### Positif (kuat untuk LinkedIn)
- Edge-type mapping benar: pointer (unwrap), `time.Time`→`date-time`, `time.Duration`→`duration`,
  `[]byte`→`string/byte`, `uuid.UUID`→`uuid`, slice-of-struct→array/object, nested struct,
  `map`→object+additionalProperties, `interface{}`→schema kosong (any), field tanpa json-tag→nama field,
  embedded struct di-flatten, `format:"uuid"`/`swagger:"required"`/`example`/`description` terbaca.
- **Adapter Gin == Chi**: `/docs/openapi.json` (Gin) dan `/docs-chi/openapi.json` (Chi) **byte-identik** (37839 B).
- UI Scalar render (theme `purple` + dark), `/docs`→`/docs/` redirect 301, spec via `./openapi.json`.
- Try-it memanggil handler asli (mis. `/showcase?filter=demo` balikin EdgeShowcase nyata).
- Output OpenAPI **3.1.0**.

## Integrasi

### Table ownership / urutan migrate (Fase 0-3)
- **go-migration** memiliki tabel app (`users`, `products`, `orders`, `order_items`)
  + tracking `migrations`. Dijalankan paling awal (`bootstrap.Fresh/Migrate`).
- **go-audit** memiliki `audit_logs`, `audit_api_logs` via `AutoMigrate` — dipanggil
  SETELAH schema app ada. Tidak ada bentrok nama.
- **go-notification** memiliki `notifications` via `migrate.Up` (Fase 5).
- Tidak ada bentrok kepemilikan/urutan selama urutannya: migrate app → audit
  AutoMigrate → notification migrate. GORM **tidak** AutoMigrate tabel app
  (model hanya memetakan), jadi satu-satunya pembuat schema app adalah go-migration.

### [INTEGRATION] Bug `WHERE WHERE` go-audit memaksa pola handler (Fase 3)
- Karena BUG `.Where(string).Update/Delete` (lihat go-audit), semua handler
  GoStore yang meng-update memakai **PK-based** `db.Model(&row).Update(...)`
  (row sudah di-`First` by id) agar snapshot audit tidak bikin SQL invalid.
  Contoh: `UploadProductImage`, mark order `paid`. Middleware `Bearer` pakai
  `First` (SELECT) sehingga aman. Tanpa workaround ini, write akan 500 karena
  `ErrorFailLoud` meng-`AddError` ke `*gorm.DB`.
- Kesimpulan: titik integrasi GORM-write × audit-plugin rapuh untuk pola
  `.Where(string)`. Perbaikan di go-audit akan menghapus batasan ini.

### txID propagation (Fase 3, lengkap di Fase 6)
- Middleware `Transaction` menaruh `txID` di `context` request (`audit.WithTransactionID`)
  + header `X-Transaction-ID`. GORM `WithContext(c.Request.Context())` membawanya ke
  audit data-change; `auditor.API().Record(ctx, ...)` membawanya ke audit API-call;
  notifikasi memakai ctx yang sama. Terbukti: header `X-Transaction-ID` muncul di
  response `POST /orders`. Korelasi data+API diverifikasi penuh di Fase 6.

### Versi Go / go.mod (Fase 0)
- Go 1.26.2. Keempat package + GORM 1.31 + pgx (gorm postgres driver) + gin +
  embedded-postgres resolve tanpa konflik `require`. Satu-satunya keanehan: adapter
  GORM go-audit hanya tersedia sebagai pseudo-version (lihat catatan Setup).

## Ringkasan — Top 5 sebelum dipromosiin

_(diisi di akhir)_
