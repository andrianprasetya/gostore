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

_(diisi Fase 1)_

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
