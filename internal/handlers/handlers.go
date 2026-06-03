// Package handlers implements GoStore's HTTP handlers (Gin). Each handler does
// real work through GORM (so go-audit captures it) and the open-swag-go
// documentation for each endpoint is co-located in docs.go.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"gostore/internal/dto"
	"gostore/internal/idgen"
	"gostore/internal/middleware"
	"gostore/internal/models"

	goaudit "github.com/gopackx/go-audit"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Notifier is the subset of go-notification GoStore handlers depend on. It is
// satisfied in internal/notify (Fase 5); nil is allowed (no-op).
type Notifier interface {
	Welcome(ctx context.Context, user models.User)
	OrderPlaced(ctx context.Context, user models.User, order models.Order)
}

// PaymentFunc simulates a third-party payment gateway call. It is provided in
// Fase 6 and records an audited API call; nil is allowed (skipped).
type PaymentFunc func(ctx context.Context, order models.Order) error

// NotificationCenter is the read side of go-notification's database channel,
// satisfied in internal/notify; nil is allowed (endpoints 503).
type NotificationCenter interface {
	Unread(ctx context.Context, user models.User) ([]dto.NotificationView, error)
	MarkAllRead(ctx context.Context, user models.User) error
}

// Handler bundles the dependencies shared by all endpoints.
type Handler struct {
	DB          *gorm.DB
	Auditor     goaudit.Auditor
	Notifier    Notifier
	NotifCenter NotificationCenter
	Payment     PaymentFunc
	AdminAPIKey string
}

func (h *Handler) notifyWelcome(ctx context.Context, u models.User) {
	if h.Notifier != nil {
		h.Notifier.Welcome(ctx, u)
	}
}

func (h *Handler) notifyOrderPlaced(ctx context.Context, u models.User, o models.Order) {
	if h.Notifier != nil {
		h.Notifier.OrderPlaced(ctx, u, o)
	}
}

// --- auth ---

// Register creates a new customer and returns a bearer token.
func (h *Handler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if len(req.Password) < 6 {
		badRequest(c, "password must be at least 6 characters")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	user := models.User{
		UUID:     idgen.NewUUID(),
		Name:     req.Name,
		Email:    req.Email,
		Password: string(hash),
		Role:     "customer",
		Active:   true,
	}
	if err := h.DB.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		badRequest(c, "could not create user (email taken?): "+err.Error())
		return
	}
	h.notifyWelcome(c.Request.Context(), user)
	c.JSON(http.StatusCreated, dto.AuthResponse{Token: itoa(user.ID), User: toUserResp(user)})
}

// Login authenticates a user by email + password.
func (h *Handler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	var user models.User
	if err := h.DB.WithContext(c.Request.Context()).Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "invalid credentials", Code: 401})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "invalid credentials", Code: 401})
		return
	}
	c.JSON(http.StatusOK, dto.AuthResponse{Token: itoa(user.ID), User: toUserResp(user)})
}

// --- products ---

// ListProducts returns published products (paginated via ?limit, ?offset).
func (h *Handler) ListProducts(c *gin.Context) {
	limit := atoiDefault(c.Query("limit"), 20)
	offset := atoiDefault(c.Query("offset"), 0)
	var products []models.Product
	q := h.DB.WithContext(c.Request.Context()).Order("id asc").Limit(limit).Offset(offset)
	if c.Query("all") != "true" {
		q = q.Where("published = ?", true)
	}
	if err := q.Find(&products).Error; err != nil {
		serverError(c, err)
		return
	}
	var total int64
	h.DB.WithContext(c.Request.Context()).Model(&models.Product{}).Count(&total)
	resp := dto.ProductListResponse{Total: total}
	for _, p := range products {
		resp.Data = append(resp.Data, toProductResp(p))
	}
	c.JSON(http.StatusOK, resp)
}

// GetProduct returns a single product by id.
func (h *Handler) GetProduct(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "invalid product id")
		return
	}
	var p models.Product
	if err := h.DB.WithContext(c.Request.Context()).First(&p, id).Error; err != nil {
		notFound(c, "product not found")
		return
	}
	c.JSON(http.StatusOK, toProductResp(p))
}

// CreateProduct creates a product (admin, bearer).
func (h *Handler) CreateProduct(c *gin.Context) {
	var req dto.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	details := req.Details
	p := models.Product{
		UUID:      idgen.NewUUID(),
		Name:      req.Name,
		SKU:       req.SKU,
		Details:   &details,
		Price:     req.Price,
		Stock:     req.Stock,
		Published: req.Published,
	}
	if err := h.DB.WithContext(c.Request.Context()).Create(&p).Error; err != nil {
		badRequest(c, "could not create product: "+err.Error())
		return
	}
	c.JSON(http.StatusCreated, toProductResp(p))
}

// UploadProductImage accepts a multipart image and records its barcode/path.
func (h *Handler) UploadProductImage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "invalid product id")
		return
	}
	var p models.Product
	if err := h.DB.WithContext(c.Request.Context()).First(&p, id).Error; err != nil {
		notFound(c, "product not found")
		return
	}
	file, err := c.FormFile("image")
	if err != nil {
		badRequest(c, "missing 'image' file field")
		return
	}
	path := "uploads/product_" + itoa(p.ID) + "_" + file.Filename
	if err := c.SaveUploadedFile(file, path); err != nil {
		serverError(c, err)
		return
	}
	// PK-based update (avoids the .Where(string).Update audit snapshot bug).
	if err := h.DB.WithContext(c.Request.Context()).Model(&p).Update("barcode", path).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "uploaded " + file.Filename})
}

// --- orders ---

// CreateOrder creates an order with nested items, charges the (mock) payment
// gateway, and fans out an OrderPlaced notification. All under one txID.
func (h *Handler) CreateOrder(c *gin.Context) {
	user := middleware.User(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "unauthorized", Code: 401})
		return
	}
	var req dto.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if len(req.Items) == 0 {
		badRequest(c, "order must have at least one item")
		return
	}
	ctx := c.Request.Context()

	order := models.Order{
		UserID:      user.ID,
		OrderNumber: "ORD-" + time.Now().Format("20060102") + "-" + goaudit.NewTransactionID()[9:15],
		Status:      "pending",
	}
	var total float64
	for _, it := range req.Items {
		var p models.Product
		if err := h.DB.WithContext(ctx).First(&p, it.ProductID).Error; err != nil {
			badRequest(c, "product not found: "+itoa(it.ProductID))
			return
		}
		if it.Quantity <= 0 {
			badRequest(c, "quantity must be positive")
			return
		}
		order.Items = append(order.Items, models.OrderItem{ProductID: p.ID, Quantity: it.Quantity, UnitPrice: p.Price})
		total += float64(it.Quantity) * p.Price
	}
	order.Total = total

	if err := h.DB.WithContext(ctx).Create(&order).Error; err != nil {
		serverError(c, err)
		return
	}

	// Mock payment (audited API call sharing the request txID).
	if h.Payment != nil {
		if err := h.Payment(ctx, order); err == nil {
			// PK-based update to mark paid (safe vs the adapter WHERE bug).
			_ = h.DB.WithContext(ctx).Model(&order).Update("status", "paid").Error
			order.Status = "paid"
		}
	}

	h.notifyOrderPlaced(ctx, *user, order)
	c.JSON(http.StatusCreated, toOrderResp(order))
}

// GetOrder returns an order (and its items) by id.
func (h *Handler) GetOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "invalid order id")
		return
	}
	var order models.Order
	if err := h.DB.WithContext(c.Request.Context()).Preload("Items").First(&order, id).Error; err != nil {
		notFound(c, "order not found")
		return
	}
	c.JSON(http.StatusOK, toOrderResp(order))
}

// AdminListOrders lists every order (API-key protected).
func (h *Handler) AdminListOrders(c *gin.Context) {
	var orders []models.Order
	if err := h.DB.WithContext(c.Request.Context()).Preload("Items").Order("id desc").Limit(100).Find(&orders).Error; err != nil {
		serverError(c, err)
		return
	}
	out := make([]dto.OrderResponse, 0, len(orders))
	for _, o := range orders {
		out = append(out, toOrderResp(o))
	}
	c.JSON(http.StatusOK, gin.H{"data": out, "total": len(out)})
}

// GetNotifications returns the authenticated user's unread in-app notifications.
func (h *Handler) GetNotifications(c *gin.Context) {
	user := middleware.User(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "unauthorized", Code: 401})
		return
	}
	if h.NotifCenter == nil {
		c.JSON(http.StatusServiceUnavailable, dto.ErrorResponse{Error: "notifications disabled", Code: 503})
		return
	}
	items, err := h.NotifCenter.Unread(c.Request.Context(), *user)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.NotificationListResponse{Data: items, Unread: len(items)})
}

// MarkNotificationsRead marks all the user's notifications as read.
func (h *Handler) MarkNotificationsRead(c *gin.Context) {
	user := middleware.User(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "unauthorized", Code: 401})
		return
	}
	if h.NotifCenter == nil {
		c.JSON(http.StatusServiceUnavailable, dto.ErrorResponse{Error: "notifications disabled", Code: 503})
		return
	}
	if err := h.NotifCenter.MarkAllRead(c.Request.Context(), *user); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "all marked read"})
}

// Showcase returns a sample EdgeShowcase so the docs try-it console renders a
// real response for the edge-type endpoint.
func (h *Handler) Showcase(c *gin.Context) {
	if c.Query("filter") == "" {
		badRequest(c, "missing required query param 'filter'")
		return
	}
	opt := "present"
	c.JSON(http.StatusOK, dto.EdgeShowcase{
		ID:       idgen.NewUUID(),
		Optional: &opt,
		When:     time.Now(),
		Items:    []dto.OrderItemResponse{{ProductID: 1, Quantity: 2, UnitPrice: 9.99}},
		Ship:     dto.ShippingAddress{Line1: "Jl. Sudirman 1", City: "Jakarta", PostalCode: "10220", Country: "ID"},
		Meta:     map[string]int{"views": 12, "likes": 3},
		Untagged: "no-json-tag-field",
		Status:   "active",
		Tags:     []string{"demo", "edge"},
	})
}

// AuditTransaction returns every data-change and API-call correlated under one
// transaction id — the core of the GoStore "one request, four packages" demo.
func (h *Handler) AuditTransaction(c *gin.Context) {
	txID := c.Param("txid")
	tl, err := h.Auditor.QueryByTransaction(c.Request.Context(), txID)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	resp := dto.TransactionResponse{TransactionID: tl.TransactionID}
	for _, d := range tl.DataLogs {
		resp.DataChanges = append(resp.DataChanges, dto.AuditDataChange{
			ID: d.ID, EntityType: d.EntityType, EntityID: d.EntityID, Action: d.Action,
			OldValues: rawToMap(d.OldValues), NewValues: rawToMap(d.NewValues),
			UserID: d.UserID, CreatedAt: d.CreatedAt,
		})
	}
	for _, a := range tl.APILogs {
		resp.APICalls = append(resp.APICalls, dto.AuditAPICall{
			ID: a.ID, Service: a.Service, Endpoint: a.Endpoint, Method: a.Method,
			StatusCode: a.StatusCode, DurationMs: a.DurationMs, CreatedAt: a.CreatedAt,
		})
	}
	resp.Summary = map[string]int{"data_changes": len(resp.DataChanges), "api_calls": len(resp.APICalls)}
	c.JSON(http.StatusOK, resp)
}

func rawToMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// --- mapping + error helpers ---

func toUserResp(u models.User) dto.UserResponse {
	return dto.UserResponse{ID: u.ID, UUID: u.UUID, Name: u.Name, Email: u.Email, Role: u.Role}
}

func toProductResp(p models.Product) dto.ProductResponse {
	return dto.ProductResponse{
		ID: p.ID, UUID: p.UUID, Name: p.Name, SKU: p.SKU, Details: p.Details,
		Price: p.Price, Stock: p.Stock, Published: p.Published, Barcode: p.Barcode, CreatedAt: p.CreatedAt,
	}
}

func toOrderResp(o models.Order) dto.OrderResponse {
	items := make([]dto.OrderItemResponse, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, dto.OrderItemResponse{ProductID: it.ProductID, Quantity: it.Quantity, UnitPrice: it.UnitPrice})
	}
	return dto.OrderResponse{
		ID: o.ID, OrderNumber: o.OrderNumber, Status: o.Status, Total: o.Total, Items: items, CreatedAt: o.CreatedAt,
	}
}

func badRequest(c *gin.Context, details string) {
	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "bad request", Code: 400, Details: details})
}
func notFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: msg, Code: 404})
}
func serverError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "internal error", Code: 500, Details: err.Error()})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func itoa(u uint64) string { return strconv.FormatUint(u, 10) }
