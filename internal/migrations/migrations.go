// Package migrations holds GoStore's struct-based schema migrations for
// go-migration. Names use the 14-digit timestamp prefix required by the
// registry's name pattern (YYYYMMDDHHMMSS_lower_snake).
package migrations

import (
	"github.com/gopackx/go-migration/pkg/migrator"
	"github.com/gopackx/go-migration/pkg/schema"
)

// Registered is the ordered list of (name, migration) pairs that make up the
// GoStore app schema. Register them in order with Register(m).
var Registered = []struct {
	Name      string
	Migration migrator.Migration
}{
	{"20260101000001_create_users_table", &CreateUsersTable{}},
	{"20260101000002_create_products_table", &CreateProductsTable{}},
	{"20260101000003_create_orders_table", &CreateOrdersTable{}},
	{"20260101000004_create_order_items_table", &CreateOrderItemsTable{}},
	{"20260101000005_alter_products_schema", &AlterProductsSchema{}},
}

// Register adds every GoStore migration to the migrator in order.
func Register(m *migrator.Migrator) error {
	for _, r := range Registered {
		if err := m.Register(r.Name, r.Migration); err != nil {
			return err
		}
	}
	return nil
}

// --- users -------------------------------------------------------------------

type CreateUsersTable struct{}

func (*CreateUsersTable) Up(s *schema.Builder) error {
	return s.Create("users", func(bp *schema.Blueprint) {
		bp.ID()
		bp.UUID("uuid")
		bp.String("name", 255)
		bp.String("email", 255).Unique()
		bp.String("password", 255) // excluded from audit via ExcludeFields
		bp.String("role", 50).Default("customer")
		bp.Text("bio").Nullable()
		bp.BigInteger("credit").Unsigned().Default(0) // Unsigned is a no-op on Postgres
		bp.JSON("preferences").Nullable()             // JSON -> JSONB on Postgres
		bp.Boolean("active").Default(true)
		bp.Timestamps()
		bp.SoftDeletes()
		bp.Index("role")
	})
}

func (*CreateUsersTable) Down(s *schema.Builder) error { return s.DropIfExists("users") }

// --- products ----------------------------------------------------------------

type CreateProductsTable struct{}

func (*CreateProductsTable) Up(s *schema.Builder) error {
	return s.Create("products", func(bp *schema.Blueprint) {
		bp.ID()
		bp.UUID("uuid")
		bp.String("name", 255)
		bp.String("sku", 100)
		bp.Text("description").Nullable() // renamed to "details" in migration 5
		bp.Decimal("price", 10, 2).Default(0)
		bp.BigInteger("stock").Unsigned().Default(0)
		bp.Boolean("published").Default(false)
		bp.JSON("attributes").Nullable()
		bp.Boolean("temp_flag").Default(false) // dropped in migration 5 (tests DropColumn)
		bp.Timestamps()
		bp.SoftDeletes()
		bp.Index("name")          // regular index -> separate CREATE INDEX statement
		bp.UniqueIndex("sku")     // unique -> table CONSTRAINT
	})
}

func (*CreateProductsTable) Down(s *schema.Builder) error { return s.DropIfExists("products") }

// --- orders ------------------------------------------------------------------

type CreateOrdersTable struct{}

func (*CreateOrdersTable) Up(s *schema.Builder) error {
	return s.Create("orders", func(bp *schema.Blueprint) {
		bp.ID()
		bp.BigInteger("user_id").Unsigned()
		bp.String("order_number", 50).Unique()
		bp.Enum("status", []string{"pending", "paid", "shipped", "cancelled"}).Default("pending")
		bp.Decimal("total", 12, 2).Default(0)
		bp.Text("notes").Nullable()
		bp.JSON("shipping_address").Nullable()
		bp.Timestamps()
		bp.SoftDeletes()
		bp.Foreign("user_id").References("id").On("users").OnDeleteAction("CASCADE")
		bp.Index("user_id")
	})
}

func (*CreateOrdersTable) Down(s *schema.Builder) error { return s.DropIfExists("orders") }

// --- order_items -------------------------------------------------------------

type CreateOrderItemsTable struct{}

func (*CreateOrderItemsTable) Up(s *schema.Builder) error {
	return s.Create("order_items", func(bp *schema.Blueprint) {
		bp.ID()
		bp.BigInteger("order_id").Unsigned()
		bp.BigInteger("product_id").Unsigned()
		bp.Integer("quantity").Default(1)
		bp.Decimal("unit_price", 10, 2).Default(0)
		bp.Timestamps()
		bp.Foreign("order_id").References("id").On("orders").OnDeleteAction("CASCADE")
		bp.Foreign("product_id").References("id").On("products").OnDeleteAction("RESTRICT")
		bp.UniqueIndex("order_id", "product_id") // combined unique index
	})
}

func (*CreateOrderItemsTable) Down(s *schema.Builder) error { return s.DropIfExists("order_items") }

// --- alter products (add + rename + drop in one migration) -------------------

type AlterProductsSchema struct{}

func (*AlterProductsSchema) Up(s *schema.Builder) error {
	return s.Alter("products", func(bp *schema.Blueprint) {
		bp.String("barcode", 64).Nullable()         // add
		bp.RenameColumn("description", "details")    // rename
		bp.DropColumn("temp_flag")                   // drop
	})
}

func (*AlterProductsSchema) Down(s *schema.Builder) error {
	return s.Alter("products", func(bp *schema.Blueprint) {
		bp.Boolean("temp_flag").Default(false)
		bp.RenameColumn("details", "description")
		bp.DropColumn("barcode")
	})
}
