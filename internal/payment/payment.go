// Package payment provides a mock payment gateway. It performs no real network
// call but records an audited API call (with redacted Authorization header and
// card number) under the request's transaction id, so a single checkout shows
// data-change + API-call correlated by one txID.
package payment

import (
	"context"
	"time"

	"gostore/internal/handlers"
	"gostore/internal/models"

	goaudit "github.com/gopackx/go-audit"
)

// MockGateway returns a handlers.PaymentFunc that "charges" an order and logs
// the call via the auditor's API channel.
func MockGateway(auditor goaudit.Auditor) handlers.PaymentFunc {
	return func(ctx context.Context, order models.Order) error {
		start := time.Now()
		// Simulate latency of a real gateway round-trip.
		time.Sleep(5 * time.Millisecond)

		return auditor.API().Record(ctx, goaudit.APIEntry{
			Service:    "payment-gateway",
			Endpoint:   "/v1/charges",
			Method:     "POST",
			StatusCode: 200,
			RequestHeaders: map[string]string{
				"Authorization": "Bearer pk_live_SUPERSECRETKEY",
				"Content-Type":  "application/json",
			},
			RequestBody: map[string]any{
				"order_number": order.OrderNumber,
				"amount":       order.Total,
				"currency":     "IDR",
				"card_number":  "4111111111111111",
				"cvv":          "123",
			},
			ResponseBody: map[string]any{"status": "succeeded", "charge_id": "ch_" + order.OrderNumber},
			DurationMs:   int(time.Since(start).Milliseconds()),
		})
	}
}
