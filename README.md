# GoStore

A realistic e-commerce backend that **dogfoods four `gopackx` packages** in one
app — to stress-test their integration points and surface bugs/friction:

| Package | Role in GoStore |
|---|---|
| [`go-migration`](https://github.com/gopackx/go-migration) | struct migrations, seeders, factories, CLI |
| [`go-audit`](https://github.com/gopackx/go-audit) | automatic data-change + API-call audit trail |
| [`go-notification`](https://github.com/gopackx/go-notification) | multi-channel in-app notifications |
| [`open-swag-go`](https://github.com/gopackx/open-swag-go) | OpenAPI docs (Scalar UI), version diff |

Stack: **PostgreSQL 16 + GORM + Gin**. `migrate → seed → serve`.

> ⭐ The most important deliverable is [`FINDINGS.md`](./FINDINGS.md) — every bug,
> DX rough edge, and integration friction found while building this.

## Run it

No Docker or local Postgres needed — GoStore boots an **embedded Postgres**
(real binary) when `DATABASE_URL` is unset. To use your own Postgres, set
`DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable`.

```bash
go run ./cmd/server         # migrate (fresh) -> seed -> serve on :8080
# docs UI:   http://localhost:8080/docs/
# spec:      http://localhost:8080/docs/openapi.json
# admin:     admin@gostore.dev / admin123   (api key: gostore-admin-key)
```

### Try the end-to-end txID correlation

```bash
TOK=$(curl -s -XPOST localhost:8080/auth/register -d '{"name":"A","email":"a@x.dev","password":"secret123"}' | jq -r .token)
curl -s -D- -XPOST localhost:8080/orders -H "Authorization: Bearer $TOK" \
  -d '{"items":[{"product_id":1,"quantity":2}]}' | grep -i X-Transaction-Id
curl -s localhost:8080/audit/transactions/<txID> -H "X-API-Key: gostore-admin-key" | jq
```

One checkout → **6 data-changes + 1 API-call** correlated under a single `transaction_id`.

## Verification harnesses

Each package has a runnable harness that prints a PASS/FAIL coverage checklist:

```bash
go run ./cmd/migrate-demo   # go-migration   — 23/23 checks
go run ./cmd/audit-demo     # go-audit        — 18/18 checks
go run ./cmd/notify-demo    # go-notification —  9/9 checks
go run ./cmd/version-diff   # open-swag-go versioning (Fase 7)
go run ./cmd/recursion-test # reproduces an open-swag-go crash (intentional)
go run ./cmd/gen-spec       # export spec/openapi.json without serving
```

## Layout

```
cmd/server          Gin app (migrate->seed->serve)        cmd/server-chi   Chi docs adapter (parity)
cmd/migrator        go-migration CLI                      cmd/gen-spec     export OpenAPI spec
cmd/*-demo          per-package verification harnesses     cmd/version-diff breaking-change diff
internal/migrations struct migrations                     internal/seed    seeders + factories
internal/models     GORM models                           internal/dto     request/response shapes
internal/handlers   handlers + co-located *Doc            internal/middleware txID + auth
internal/audit      go-audit setup                        internal/notify  go-notification setup
internal/docs       open-swag-go setup                    internal/payment mock gateway (audited)
spec/               openapi-v1.json, openapi-v2.json      frontend-types/  generated api.d.ts
FINDINGS.md         ⭐ main deliverable                    linkedin-assets/ story material
```

## DB strategy (one pool)

GORM opens the connection; we pull `*sql.DB` via `gormDB.DB()` and hand the **same
pool** to go-migration (`PostgresGrammar`). go-audit's `AutoMigrate` and
go-notification's `migrate.Up` run on that pool too. Ownership: go-migration owns
app tables, go-audit owns `audit_*`, go-notification owns `notifications`.

## License

MIT.
