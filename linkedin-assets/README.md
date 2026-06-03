# LinkedIn assets — GoStore (dogfooding the `gopackx` stack)

> Bahan mentah untuk post "Laravel-style DX untuk Go". Caption **tidak** ditulis
> di sini (disusun terpisah setelah tahu hasil nyata). Semua di bawah berasal dari
> run sungguhan di PostgreSQL 16 asli (embedded-postgres, tanpa Docker).

## Angka headline

| Metrik | Nilai |
|---|---|
| Baris YAML/JSON OpenAPI ditulis tangan | **0** (semua dari struct Go) |
| Tabel ter-migrate via struct (bukan SQL file) | **4** app + tracking `migrations` |
| Tabel total yang "tinggal jalan" | 8 (app 4 + `migrations` + `audit_logs` + `audit_api_logs` + `notifications`) |
| Endpoint terdokumentasi otomatis | **12** (co-located `*Doc`) |
| Baris TypeScript ter-generate | **885** (`frontend-types/api.d.ts`) |
| Adapter docs identik | Gin == Chi (spec byte-identik) |
| 1 checkout → audit terkorelasi | **6 data-change + 1 API-call** di bawah 1 `transaction_id` |
| Cek verifikasi otomatis (3 harness) | 23 + 18 + 9 = **50** hijau |

## Aset 1 — Dokumentasi API auto-generated (Scalar UI)

UI Scalar (theme purple + dark, sidebar, tag-grouping) render dari struct Go,
0 baris YAML. Endpoint dikelompokkan per tag: Auth, Products, Orders, Admin,
Notifications, Audit, Showcase. (Screenshot UI bisa diambil dari `http://localhost:8080/docs/`
saat `cmd/server` jalan — UI terkonfirmasi render.)

Cuplikan co-located doc (DX ala decorator):

```go
var CreateOrderDoc = openswag.Endpoint{
    Method: "POST", Path: "/orders", Summary: "Place an order (nested items)",
    Tags: []string{"Orders"}, Security: []string{openswag.SecurityBearerAuth},
    RequestBody: jsonBody("Order with line items", dto.CreateOrderRequest{}, true),
    Responses: map[int]openswag.Response{
        201: {Description: "Created", Schema: dto.OrderResponse{}},
        401: {Description: "Unauthorized", Schema: dto.ErrorResponse{}},
    },
}
```

## Aset 2 — Korelasi `transaction_id` (PALING KUAT)

Satu `POST /orders` → middleware bikin `txID` → GORM simpan order (go-audit catat
data-change) → panggil mock payment (go-audit catat API-call, header & kartu
ter-redaksi) → go-notification kirim OrderPlaced. Lalu `GET /audit/transactions/{txID}`
mengikat semuanya:

```
txID: 20260603T...-f3419dad...
summary: { "data_changes": 6, "api_calls": 1 }
```

→ `linkedin-assets/raw/txid-correlation.json` (full), `raw/notifications.json`.
Empat package jalan dalam satu request, terikat satu id.

## Aset 3 — Snapshot & Restore ("time travel")

```
[PASS] Snapshot reconstructs past state — price@midpoint=20 (expected 20)
[PASS] Restore records action=restore — restore entry written, target price=20
```

Ubah harga produk beberapa kali → `Snapshot(ctx,"products",id,kemarin)` merekonstruksi
harga lampau → `Restore(...)` menulis entry `action: restore`. → `raw/audit-highlights.txt`.

## Aset 4 — Migration + seeder/factory (DX ala Laravel)

```go
func (*CreateProductsTable) Up(s *schema.Builder) error {
    return s.Create("products", func(bp *schema.Blueprint) {
        bp.ID()
        bp.UUID("uuid")
        bp.String("name", 255)
        bp.Decimal("price", 10, 2).Default(0)
        bp.JSON("attributes").Nullable()   // -> jsonb di Postgres
        bp.Timestamps(); bp.SoftDeletes()
        bp.Index("name"); bp.UniqueIndex("sku")
    })
}

// Factory + faker + named state
f := factory.NewFactory(func(fk factory.Faker) UserRow { /* ... */ })
f.State("admin", func(fk factory.Faker, u UserRow) UserRow { u.Role = "admin"; return u })
admin := f.WithState("admin").Make()
users := f.MakeMany(20)
```

`Up/Status/Rollback(batch+step)/Reset/Refresh/Fresh`, hooks durasi, deteksi
circular-dependency, dan opt-out transaksi (`DisableTransaction`) semua terbukti
(`cmd/migrate-demo`, 23/23 hijau).

## Aset 5 — Notifikasi fan-out multi-channel

```go
func (OrderPlacedNotification) Via(notification.Notifiable) []string {
    return []string{"database", "test"} // in-app + custom channel, tanpa kredensial asli
}
func (o OrderPlacedNotification) ToDatabase(notification.Notifiable) *database.Message {
    return database.NewMessage().SetType("order.placed").
        SetTitle("Order received").SetBody("We got your order " + o.OrderNumber)
}
```

Async worker-pool thread-safe (50 kirim konkuren semua persist), `Sync()` blocking,
`Via()` override, `OnError` setelah retry, `Close()` drain — `cmd/notify-demo` 9/9 hijau.

## Aset 6 — Version diff (deteksi breaking change)

```
Breaking changes: true
Summary: added=0 removed=1 modified=2 breaking=4
  [get /showcase] Endpoint removed
  [post /products] New required field: currency
  [post /products] New required parameter: reason
  [get /products/{id}] Response code 404 removed
```

→ `linkedin-assets/raw/version-diff.txt` (lengkap + changelog markdown).

## Aset 7 — Full-stack: Go struct → OpenAPI → TypeScript

`openapi-typescript` menghasilkan tipe TS dari spec; semua edge-type kebawa tepat
(`uuid`, `map`→`Record`, nested, slice-of-struct, `[]byte`→`byte`). → `frontend-types/api.d.ts`.

---

### Catatan jujur (kredibilitas post)

Dogfooding ini juga menemukan bug nyata (ada di `FINDINGS.md`) — mis. go-audit
`WHERE WHERE` pada `.Where(string).Update`, dan README open-swag-go yang belum
sinkron dgn API. Post yang menyebut temuan + perbaikan biasanya lebih dipercaya
ketimbang yang cuma memuji.
