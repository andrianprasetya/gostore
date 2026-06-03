package handlers

import (
	"gostore/internal/dto"

	openswag "github.com/gopackx/open-swag-go"
)

// Co-located open-swag-go documentation for each endpoint.
//
// NOTE: this uses the ACTUAL v1.1.1 API (struct literals). The README's helper
// functions (openswag.Body/Response/Responses/BearerAuth/AuthConfig/PathParam…)
// do NOT exist in v1.1.1 — see FINDINGS. The real shapes are:
//   RequestBody: &openswag.RequestBody{Schema: T{}, ...}
//   Responses:   map[int]openswag.Response{201: {Description: "...", Schema: T{}}}
//   Parameters:  []openswag.Parameter{{Name, In, Required, Description}}
//   Security:    []string{openswag.SecurityBearerAuth}

func jsonBody(desc string, schema any, required bool) *openswag.RequestBody {
	return &openswag.RequestBody{Description: desc, Required: required, Schema: schema, ContentType: "application/json"}
}

var RegisterDoc = openswag.Endpoint{
	Method: "POST", Path: "/auth/register", Summary: "Register a new customer",
	Tags: []string{"Auth"}, RequestBody: jsonBody("Registration payload", dto.RegisterRequest{}, true),
	Responses: map[int]openswag.Response{
		201: {Description: "Created", Schema: dto.AuthResponse{}},
		400: {Description: "Validation error", Schema: dto.ErrorResponse{}},
	},
}

var LoginDoc = openswag.Endpoint{
	Method: "POST", Path: "/auth/login", Summary: "Login with email + password",
	Tags: []string{"Auth"}, RequestBody: jsonBody("Credentials", dto.LoginRequest{}, true),
	Responses: map[int]openswag.Response{
		200: {Description: "OK", Schema: dto.AuthResponse{}},
		401: {Description: "Invalid credentials", Schema: dto.ErrorResponse{}},
	},
}

var ListProductsDoc = openswag.Endpoint{
	Method: "GET", Path: "/products", Summary: "List published products", Tags: []string{"Products"},
	Parameters: []openswag.Parameter{
		{Name: "limit", In: "query", Description: "Page size (default 20)"},
		{Name: "offset", In: "query", Description: "Page offset (default 0)"},
		{Name: "all", In: "query", Description: "Include unpublished (true/false)"},
		{Name: "X-Request-ID", In: "header", Description: "Optional client trace id"},
	},
	Responses: map[int]openswag.Response{200: {Description: "OK", Schema: dto.ProductListResponse{}}},
}

var GetProductDoc = openswag.Endpoint{
	Method: "GET", Path: "/products/{id}", Summary: "Get a product by id", Tags: []string{"Products"},
	Parameters: []openswag.Parameter{{Name: "id", In: "path", Required: true, Description: "Product id"}},
	Responses: map[int]openswag.Response{
		200: {Description: "OK", Schema: dto.ProductResponse{}},
		404: {Description: "Not found", Schema: dto.ErrorResponse{}},
	},
}

var CreateProductDoc = openswag.Endpoint{
	Method: "POST", Path: "/products", Summary: "Create a product (admin)", Tags: []string{"Products"},
	Security: []string{openswag.SecurityBearerAuth}, RequestBody: jsonBody("Product to create", dto.CreateProductRequest{}, true),
	Responses: map[int]openswag.Response{
		201: {Description: "Created", Schema: dto.ProductResponse{}},
		401: {Description: "Unauthorized", Schema: dto.ErrorResponse{}},
		403: {Description: "Forbidden (not admin)", Schema: dto.ErrorResponse{}},
	},
}

var UploadProductImageDoc = openswag.Endpoint{
	Method: "POST", Path: "/products/{id}/image", Summary: "Upload a product image (multipart)",
	Tags: []string{"Products"}, Security: []string{openswag.SecurityBearerAuth},
	Parameters: []openswag.Parameter{{Name: "id", In: "path", Required: true, Description: "Product id"}},
	RequestBody: &openswag.RequestBody{
		Description: "multipart/form-data with an 'image' file field", Required: true,
		ContentType: "multipart/form-data", Schema: struct {
			Image string `json:"image" format:"binary" description:"Image file"`
		}{},
	},
	Responses: map[int]openswag.Response{
		200: {Description: "Uploaded", Schema: dto.MessageResponse{}},
		404: {Description: "Not found", Schema: dto.ErrorResponse{}},
	},
}

var CreateOrderDoc = openswag.Endpoint{
	Method: "POST", Path: "/orders", Summary: "Place an order (nested items)", Tags: []string{"Orders"},
	Security: []string{openswag.SecurityBearerAuth}, RequestBody: jsonBody("Order with line items", dto.CreateOrderRequest{}, true),
	Responses: map[int]openswag.Response{
		201: {Description: "Created", Schema: dto.OrderResponse{}},
		400: {Description: "Bad request", Schema: dto.ErrorResponse{}},
		401: {Description: "Unauthorized", Schema: dto.ErrorResponse{}},
	},
}

var GetOrderDoc = openswag.Endpoint{
	Method: "GET", Path: "/orders/{id}", Summary: "Get an order by id", Tags: []string{"Orders"},
	Security: []string{openswag.SecurityBearerAuth},
	Parameters: []openswag.Parameter{{Name: "id", In: "path", Required: true, Description: "Order id"}},
	Responses: map[int]openswag.Response{
		200: {Description: "OK", Schema: dto.OrderResponse{}},
		404: {Description: "Not found", Schema: dto.ErrorResponse{}},
	},
}

var AdminListOrdersDoc = openswag.Endpoint{
	Method: "GET", Path: "/admin/orders", Summary: "List all orders (service / API key)", Tags: []string{"Admin"},
	Security: []string{openswag.SecurityApiKey},
	Responses: map[int]openswag.Response{
		200: {Description: "OK", Schema: dto.OrderResponse{}},
		401: {Description: "Invalid API key", Schema: dto.ErrorResponse{}},
	},
}

var ShowcaseDoc = openswag.Endpoint{
	Method: "GET", Path: "/showcase", Summary: "Edge-type schema showcase", Tags: []string{"Showcase"},
	Description: "Exercises pointer / time.Time / slice-of-struct / nested / map / no-tag / enum / uuid / []byte.",
	Security:    []string{openswag.SecurityBasicAuth, openswag.SecurityApiKeyQuery},
	Parameters: []openswag.Parameter{
		{Name: "filter", In: "query", Required: true, Description: "Required query param"},
	},
	Responses: map[int]openswag.Response{
		200: {Description: "OK", Schema: dto.EdgeShowcase{}},
		500: {Description: "Server error", Schema: dto.ErrorResponse{}},
	},
}

var GetNotificationsDoc = openswag.Endpoint{
	Method: "GET", Path: "/notifications", Summary: "List my unread in-app notifications",
	Tags: []string{"Notifications"}, Security: []string{openswag.SecurityBearerAuth},
	Responses: map[int]openswag.Response{
		200: {Description: "OK", Schema: dto.NotificationListResponse{}},
		401: {Description: "Unauthorized", Schema: dto.ErrorResponse{}},
	},
}

var MarkNotificationsReadDoc = openswag.Endpoint{
	Method: "POST", Path: "/notifications/read-all", Summary: "Mark all my notifications read",
	Tags: []string{"Notifications"}, Security: []string{openswag.SecurityBearerAuth},
	Responses: map[int]openswag.Response{
		200: {Description: "OK", Schema: dto.MessageResponse{}},
		401: {Description: "Unauthorized", Schema: dto.ErrorResponse{}},
	},
}

// Endpoints is the full co-located set, registered via docs.Setup -> AddAll.
var Endpoints = []openswag.Endpoint{
	RegisterDoc, LoginDoc,
	ListProductsDoc, GetProductDoc, CreateProductDoc, UploadProductImageDoc,
	CreateOrderDoc, GetOrderDoc, AdminListOrdersDoc,
	GetNotificationsDoc, MarkNotificationsReadDoc, ShowcaseDoc,
}
