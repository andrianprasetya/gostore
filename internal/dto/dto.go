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

// --- notifications (in-app) ---

type NotificationView struct {
	ID        string         `json:"id" example:"abc123"`
	Type      string         `json:"type" example:"order.placed"`
	Title     string         `json:"title" example:"Order received"`
	Body      string         `json:"body" example:"We got your order ORD-2026-0001"`
	Data      map[string]any `json:"data,omitempty"`
	Read      bool           `json:"read" example:"false"`
	CreatedAt time.Time      `json:"created_at" format:"date-time"`
}

type NotificationListResponse struct {
	Data   []NotificationView `json:"data"`
	Unread int                `json:"unread" example:"2"`
}

// --- audit (txID correlation showcase) ---

type AuditDataChange struct {
	ID         uint64         `json:"id"`
	EntityType string         `json:"entity_type" example:"orders"`
	EntityID   string         `json:"entity_id" example:"12"`
	Action     string         `json:"action" example:"create" description:"create|update|delete|soft_delete|restore"`
	OldValues  map[string]any `json:"old_values,omitempty"`
	NewValues  map[string]any `json:"new_values,omitempty"`
	UserID     string         `json:"user_id" example:"42"`
	CreatedAt  time.Time      `json:"created_at" format:"date-time"`
}

type AuditAPICall struct {
	ID         uint64    `json:"id"`
	Service    string    `json:"service" example:"payment-gateway"`
	Endpoint   string    `json:"endpoint" example:"/v1/charges"`
	Method     string    `json:"method" example:"POST"`
	StatusCode int       `json:"status_code" example:"200"`
	DurationMs int       `json:"duration_ms" example:"6"`
	CreatedAt  time.Time `json:"created_at" format:"date-time"`
}

type TransactionResponse struct {
	TransactionID string            `json:"transaction_id" example:"20260603T050002-..."`
	DataChanges   []AuditDataChange `json:"data_changes"`
	APICalls      []AuditAPICall    `json:"api_calls"`
	Summary       map[string]int    `json:"summary" description:"counts per kind"`
}

// --- doc edge-type showcase (Fase 4) ---

// EdgeShowcase deliberately exercises the schema generator's tricky cases:
// pointer, time.Time, slice-of-struct, nested struct, map, a field with no
// json tag, an enum-ish field, and a uuid format hint.
type EdgeShowcase struct {
	ID         string              `json:"id" format:"uuid" description:"uuid format hint"`
	Optional   *string             `json:"optional,omitempty" description:"pointer -> nullable-ish"`
	When       time.Time           `json:"when" description:"time.Time -> string/date-time"`
	Items      []OrderItemResponse `json:"items" description:"slice of struct"`
	Ship       ShippingAddress     `json:"ship" description:"nested struct"`
	Meta       map[string]int      `json:"meta" description:"map -> additionalProperties"`
	Untagged   string              `description:"no json tag -> field name used"`
	Status     string              `json:"status" swagger:"required" example:"active" description:"enum-ish; one of active|inactive"`
	Tags       []string            `json:"tags" example:"a"`
	RawPayload []byte              `json:"raw_payload" description:"[]byte -> string/byte"`
}

// Category is a self-referential type used ONLY by the isolated recursion test
// (cmd/recursion-test). Do NOT add it to the served spec — the v1.1.1 schema
// generator recurses without a visited-set and stack-overflows on it.
type Category struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Children []Category `json:"children"`
}
