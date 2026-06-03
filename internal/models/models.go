// Package models holds GoStore's GORM models. They map onto the tables created
// by go-migration (internal/migrations), not the other way around — GORM never
// AutoMigrates the app schema here.
package models

import (
	"time"

	"gorm.io/gorm"
)

// User maps to the users table.
type User struct {
	ID          uint64         `gorm:"primaryKey" json:"id"`
	UUID        string         `gorm:"column:uuid" json:"uuid"`
	Name        string         `json:"name"`
	Email       string         `gorm:"uniqueIndex" json:"email"`
	Password    string         `json:"-"` // excluded from audit via ExcludeFields too
	Role        string         `json:"role"`
	Bio         *string        `json:"bio,omitempty"`
	Credit      int64          `json:"credit"`
	Preferences *string        `gorm:"type:jsonb" json:"preferences,omitempty"`
	Active      bool           `json:"active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (User) TableName() string { return "users" }

// Product maps to the products table (post-alter: details, barcode; no temp_flag).
type Product struct {
	ID         uint64         `gorm:"primaryKey" json:"id"`
	UUID       string         `gorm:"column:uuid" json:"uuid"`
	Name       string         `json:"name"`
	SKU        string         `gorm:"column:sku" json:"sku"`
	Details    *string        `json:"details,omitempty"`
	Price      float64        `gorm:"type:decimal(10,2)" json:"price"`
	Stock      int64          `json:"stock"`
	Published  bool           `json:"published"`
	Attributes *string        `gorm:"type:jsonb" json:"attributes,omitempty"`
	Barcode    *string        `json:"barcode,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Product) TableName() string { return "products" }

// Order maps to the orders table.
type Order struct {
	ID              uint64         `gorm:"primaryKey" json:"id"`
	UserID          uint64         `json:"user_id"`
	OrderNumber     string         `json:"order_number"`
	Status          string         `json:"status"`
	Total           float64        `gorm:"type:decimal(12,2)" json:"total"`
	Notes           *string        `json:"notes,omitempty"`
	ShippingAddress *string        `gorm:"type:jsonb" json:"shipping_address,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	Items           []OrderItem    `gorm:"foreignKey:OrderID" json:"items,omitempty"`
}

func (Order) TableName() string { return "orders" }

// OrderItem maps to the order_items table.
type OrderItem struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	OrderID   uint64    `json:"order_id"`
	ProductID uint64    `json:"product_id"`
	Quantity  int       `json:"quantity"`
	UnitPrice float64   `gorm:"type:decimal(10,2)" json:"unit_price"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (OrderItem) TableName() string { return "order_items" }
