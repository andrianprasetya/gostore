// Package seed contains GoStore's seeders and factories for go-migration.
package seed

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/gopackx/go-migration/pkg/seeder"
	"github.com/gopackx/go-migration/pkg/seeder/factory"
)

// --- factory row types -------------------------------------------------------

type UserRow struct {
	UUID        string
	Name        string
	Email       string
	Password    string
	Role        string
	Bio         string
	Credit      int
	Preferences map[string]any
	Active      bool
}

type ProductRow struct {
	UUID        string
	Name        string
	SKU         string
	Details     string
	Price       float64
	Stock       int
	Published   bool
	Attributes  map[string]any
}

// UserFactory builds realistic users. The "admin" state promotes role + name.
func UserFactory() *factory.Factory[UserRow] {
	f := factory.NewFactory(func(fake factory.Faker) UserRow {
		return UserRow{
			UUID:     fake.UUID(),
			Name:     fake.Name(),
			Email:    fake.Email(),
			Password: "secret-" + fake.Word(),
			Role:     "customer",
			Bio:      fake.Sentence(),
			Credit:   fake.IntBetween(0, 100000),
			Preferences: map[string]any{
				"newsletter": fake.Bool(),
				"theme":      fake.Pick([]string{"light", "dark", "system"}),
			},
			Active: fake.Bool(),
		}
	})
	f.State("admin", func(fake factory.Faker, base UserRow) UserRow {
		base.Name = "Admin " + base.Name
		base.Role = "admin"
		base.Active = true
		return base
	})
	return f
}

// ProductFactory builds realistic products.
func ProductFactory() *factory.Factory[ProductRow] {
	return factory.NewFactory(func(fake factory.Faker) ProductRow {
		return ProductRow{
			UUID:      fake.UUID(),
			Name:      fake.Word() + " " + fake.Word(),
			SKU:       fmt.Sprintf("SKU-%s", fake.UUID()[:8]),
			Details:   fake.Paragraph(),
			Price:     fake.Float64Between(5, 5000),
			Stock:     fake.IntBetween(0, 500),
			Published: fake.Bool(),
			Attributes: map[string]any{
				"color": fake.Pick([]string{"red", "green", "blue", "black"}),
				"size":  fake.Pick([]string{"S", "M", "L", "XL"}),
			},
		}
	})
}

// --- seeders -----------------------------------------------------------------

// UserSeeder inserts one admin + 20 customers built via UserFactory.
type UserSeeder struct{}

func (*UserSeeder) Run(db *sql.DB) error {
	f := UserFactory()
	rows := append([]UserRow{f.WithState("admin").Make()}, f.MakeMany(20)...)
	for i, u := range rows {
		prefs, _ := json.Marshal(u.Preferences)
		// Force unique emails (faker may collide across 21 rows).
		email := fmt.Sprintf("u%d_%s", i, u.Email)
		_, err := db.Exec(
			`INSERT INTO users (uuid, name, email, password, role, bio, credit, preferences, active)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			u.UUID, u.Name, email, u.Password, u.Role, u.Bio, u.Credit, string(prefs), u.Active,
		)
		if err != nil {
			return fmt.Errorf("insert user %d: %w", i, err)
		}
	}
	return nil
}

// ProductSeeder inserts 15 products built via ProductFactory.
type ProductSeeder struct{}

func (*ProductSeeder) Run(db *sql.DB) error {
	for i, p := range ProductFactory().MakeMany(15) {
		attrs, _ := json.Marshal(p.Attributes)
		_, err := db.Exec(
			`INSERT INTO products (uuid, name, sku, details, price, stock, published, attributes)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			p.UUID, p.Name, fmt.Sprintf("%s-%d", p.SKU, i), p.Details, p.Price, p.Stock, p.Published, string(attrs),
		)
		if err != nil {
			return fmt.Errorf("insert product %d: %w", i, err)
		}
	}
	return nil
}

// OrderSeeder creates an order per (some) users with 1-3 items. Declares
// dependencies on UserSeeder and ProductSeeder so the runner orders correctly.
type OrderSeeder struct{}

func (*OrderSeeder) DependsOn() []string { return []string{"UserSeeder", "ProductSeeder"} }

func (*OrderSeeder) Run(db *sql.DB) error {
	userIDs, err := ids(db, "SELECT id FROM users LIMIT 10")
	if err != nil {
		return err
	}
	type prod struct {
		id    int64
		price float64
	}
	rows, err := db.Query("SELECT id, price FROM products LIMIT 10")
	if err != nil {
		return err
	}
	var products []prod
	for rows.Next() {
		var p prod
		if err := rows.Scan(&p.id, &p.price); err != nil {
			rows.Close()
			return err
		}
		products = append(products, p)
	}
	rows.Close()
	if len(userIDs) == 0 || len(products) == 0 {
		return fmt.Errorf("OrderSeeder: need users and products seeded first")
	}

	for i, uid := range userIDs {
		var orderID int64
		err := db.QueryRow(
			`INSERT INTO orders (user_id, order_number, status, total)
			 VALUES ($1,$2,$3,$4) RETURNING id`,
			uid, fmt.Sprintf("ORD-%05d", i+1), "pending", 0,
		).Scan(&orderID)
		if err != nil {
			return fmt.Errorf("insert order %d: %w", i, err)
		}
		// 1-2 distinct items per order.
		total := 0.0
		n := 1 + (i % 2)
		for j := 0; j < n && j < len(products); j++ {
			p := products[(i+j)%len(products)]
			qty := 1 + (j % 3)
			if _, err := db.Exec(
				`INSERT INTO order_items (order_id, product_id, quantity, unit_price)
				 VALUES ($1,$2,$3,$4)`,
				orderID, p.id, qty, p.price,
			); err != nil {
				return fmt.Errorf("insert order_item: %w", err)
			}
			total += float64(qty) * p.price
		}
		if _, err := db.Exec(`UPDATE orders SET total=$1 WHERE id=$2`, total, orderID); err != nil {
			return err
		}
	}
	return nil
}

// Register adds the three core seeders to a registry.
func Register(reg *seeder.Registry) error {
	for name, s := range map[string]seeder.Seeder{
		"UserSeeder":    &UserSeeder{},
		"ProductSeeder": &ProductSeeder{},
		"OrderSeeder":   &OrderSeeder{},
	} {
		if err := reg.Register(name, s); err != nil {
			return err
		}
	}
	return nil
}

func ids(db *sql.DB, q string) ([]int64, error) {
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// --- circular dependency demo (NOT registered in the core registry) ----------

// CircularA and CircularB depend on each other to exercise the runner's
// circular-dependency detection. Used only by the Fase 1 demo.
type CircularA struct{}

func (*CircularA) Run(*sql.DB) error      { return nil }
func (*CircularA) DependsOn() []string { return []string{"CircularB"} }

type CircularB struct{}

func (*CircularB) Run(*sql.DB) error      { return nil }
func (*CircularB) DependsOn() []string { return []string{"CircularA"} }
