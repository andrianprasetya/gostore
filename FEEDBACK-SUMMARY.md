# Feedback Summary — dogfooding `gopackx` (GoStore)

> Ringkasan eksekutif untuk update feedback ke maintainer keempat package.
> Detail + repro lengkap ada di [`FINDINGS.md`](./FINDINGS.md); transkrip uji di
> [`verification/`](./verification/). Semua diuji di PostgreSQL 16 asli
> (embedded-postgres, tanpa Docker).

## Verifikasi (3 harness, 50 cek hijau)

- **`cmd/migrate-demo` — go-migration 23/23**
  (Up/Status/Rollback batch+step/Reset/Refresh/Fresh, hooks, circular-dep,
  factory states, `DisableTransaction` opt-out benar-benar beda perilaku)
- **`cmd/audit-demo` — go-audit 18/18**
  (capture CUD+diff, ExcludeFields, soft/hard delete, redaksi header+body+truncation,
  korelasi txID, snapshot/restore, purge, ExcludeEntities)
- **`cmd/notify-demo` — go-notification 9/9**
  (fan-out async thread-safe, Sync, Via, OnError, Close, rate-limit)
- **E2E nyata via HTTP:** 1 checkout → **6 data-change + 1 API-call di bawah satu
  `transaction_id`** + notifikasi in-app
- **Version-diff:** 4 breaking change terdeteksi; **TS type-gen 885 baris tanpa
  mismatch**; docs UI Scalar terkonfirmasi render (**Gin == Chi byte-identik**)

## Temuan paling berharga (Top 5)

1. **BUG · go-audit** — `.Where("…").Update/Delete` bikin SQL `WHERE WHERE`
   (SQLSTATE 42601) yang **membatalkan tulisan** (pola GORM paling umum). Repro deterministik.
2. **DOCS · open-swag-go** — README mendokumentasikan API yang tak ada di v1.1.1
   (Quick Start tak compile).
3. **BUG · open-swag-go** — tipe struct self-referential → **stack overflow** saat generate spec.
4. **BUG · go-audit** — nested `Create` menggandakan audit row anak (2 item → 4 row).
5. **BUG · open-swag-go** — `RequestBody.ContentType` diabaikan (multipart tak terdokumentasi).

Plus temuan minor: `OnDelete` vs `OnDeleteAction` (README go-migration), `make:seeder`
tak auto-register, version-diff tak deteksi perubahan tipe field, `RedactBodyFields`
lewat body struct, `old_values` create tersimpan sebagai literal `null`, dll.

## Verdict per package

| Package | Verdict |
|---|---|
| **go-notification** | Paling matang, nyaris tanpa friksi. Siap dipromosikan. |
| **go-migration** | Solid; temuan mayoritas DOCS/DX kecil. Hampir siap. |
| **go-audit** | Fitur kuat (korelasi txID, time-travel), tapi **2 BUG GORM-adapter** harus beres dulu. |
| **open-swag-go** | Engine schema & UI bagus (edge-type→TS mulus, Gin==Chi), tapi **README menyesatkan + 2 BUG** (recursion, ContentType). |
