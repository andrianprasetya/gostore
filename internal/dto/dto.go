// Package dto holds request/response shapes for the GoStore API. Struct tags
// double as open-swag-go documentation hints (swagger/example/format/...).
package dto

import "time"

// --- auth ---

type RegisterRequest struct {
	Name     string `json:"name" swagger:"required" example:"Andrian Prasetya" description:"Full name"`
	Email    string `json:"email" swagger:"required,format=email" example:"andrian@example.com"`
	Password string `json:"password" swagger:"required" example:"s3cret-pass" description:"Min 6 chars"`
}

type LoginRequest struct {
	Email    string `json:"email" swagger:"required,format=email" example:"andrian@example.com"`
	Password string `json:"password" swagger:"required" example:"s3cret-pass"`
}

type AuthResponse struct {
	Token string       `json:"token" example:"42" description:"Bearer token for subsequent requests"`
	User  UserResponse `json:"user"`
}

type UserResponse struct {
	ID    uint64 `json:"id" example:"42"`
	UUID  string `json:"uuid" format:"uuid"`
	Name  string `json:"name" example:"Andrian Prasetya"`
	Email string `json:"email" format:"email"`
	Role  string `json:"role" example:"customer" description:"customer | admin"`
}

// --- products ---

type CreateProductRequest struct {
	Name      string   `json:"name" swagger:"required" example:"Mechanical Keyboard"`
	SKU       string   `json:"sku" swagger:"required" example:"KB-001"`
	Details   string   `json:"details" example:"Hot-swappable, 75%"`
	Price     float64  `json:"price" swagger:"required" example:"129.99"`
	Stock     int64    `json:"stock" example:"50"`
	Published bool     `json:"published" example:"true"`
	Tags      []string `json:"tags" example:"mechanical" description:"Free-form tags"`
}

type ProductResponse struct {
	ID        uint64    `json:"id" example:"1"`
	UUID      string    `json:"uuid" format:"uuid"`
	Name      string    `json:"name" example:"Mechanical Keyboard"`
	SKU       string    `json:"sku" example:"KB-001"`
	Details   *string   `json:"details,omitempty"`
	Price     float64   `json:"price" example:"129.99"`
	Stock     int64     `json:"stock" example:"50"`
	Published bool      `json:"published" example:"true"`
	Barcode   *string   `json:"barcode,omitempty"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
}

type ProductListResponse struct {
	Data  []ProductResponse `json:"data"`
	Total int64             `json:"total" example:"42"`
}

// --- orders ---

type CreateOrderItem struct {
	ProductID uint64 `json:"product_id" swagger:"required" example:"1"`
	Quantity  int    `json:"quantity" swagger:"required" example:"2"`
}

type CreateOrderRequest struct {
	Items           []CreateOrderItem `json:"items" swagger:"required" description:"Order line items"`
	ShippingAddress *ShippingAddress  `json:"shipping_address,omitempty"`
}

type ShippingAddress struct {
	Line1      string `json:"line1" example:"Jl. Sudirman 1"`
	City       string `json:"city" example:"Jakarta"`
	PostalCode string `json:"postal_code" example:"10220"`
	Country    string `json:"country" example:"ID"`
}

type OrderItemResponse struct {
	ProductID uint64  `json:"product_id" example:"1"`
	Quantity  int     `json:"quantity" example:"2"`
	UnitPrice float64 `json:"unit_price" example:"129.99"`
}

type OrderResponse struct {
	ID          uint64              `json:"id" example:"1"`
	OrderNumber string              `json:"order_number" example:"ORD-2026-0001"`
	Status      string              `json:"status" example:"paid" description:"pending | paid | shipped | cancelled"`
	Total       float64             `json:"total" example:"259.98"`
	Items       []OrderItemResponse `json:"items"`
	CreatedAt   time.Time           `json:"created_at" format:"date-time"`
}

// --- shared ---

type ErrorResponse struct {
	Error   string `json:"error" example:"not found"`
	Code    int    `json:"code" example:"404"`
	Details string `json:"details,omitempty"`
}

type MessageResponse struct {
	Message string `json:"message" example:"ok"`
}
