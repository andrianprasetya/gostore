# Verification transcripts

Captured PASS/FAIL output from the per-package harnesses (real PostgreSQL 16 via
embedded-postgres). Reproduce any of these with `go run ./cmd/<name>`.

| File | Harness | Result |
|---|---|---|
| `fase1-go-migration.txt` | `cmd/migrate-demo` | **23/23 PASS** |
| `fase2-go-audit.txt` | `cmd/audit-demo` | **18/18 PASS** |
| `fase5-go-notification.txt` | `cmd/notify-demo` | **9/9 PASS** |
| `fase7-version-diff.txt` | `cmd/version-diff` | 4 breaking changes detected |

E2E txID-correlation output (Fase 6) and full version-diff/changelog live in
`../linkedin-assets/raw/`. The intentional crash repro is `cmd/recursion-test`
(open-swag-go stack overflow on recursive types). Findings are in `../FINDINGS.md`.
