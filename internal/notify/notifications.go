package notify

import (
	"context"
	"log"
	"time"

	notification "github.com/gopackx/go-notification"
	"github.com/gopackx/go-notification/channel/database"
)

func secondsToDur(s int) time.Duration { return time.Duration(s) * time.Second }

// --- notifications ---

// WelcomeNotification fans out to the in-app database channel and the test
// channel. It implements:
//   - database.Notification (ToDatabase) for the database channel's Formatter
//   - notification.ChannelFormatter (Format) for the custom "test" channel
type WelcomeNotification struct{ Name string }

func (WelcomeNotification) Via(notification.Notifiable) []string { return []string{"database", "test"} }

func (w WelcomeNotification) ToDatabase(notification.Notifiable) *database.Message {
	return database.NewMessage().
		SetType("user.welcome").
		SetTitle("Welcome to GoStore!").
		SetBody("Hi " + w.Name + ", thanks for signing up.").
		AddData("name", w.Name)
}

func (w WelcomeNotification) Format(channel string, _ notification.Notifiable) any {
	return map[string]any{"kind": "welcome", "name": w.Name}
}

// OrderPlacedNotification fans out to database + test channels.
type OrderPlacedNotification struct {
	OrderNumber string
	Total       float64
}

func (OrderPlacedNotification) Via(notification.Notifiable) []string { return []string{"database", "test"} }

func (o OrderPlacedNotification) ToDatabase(notification.Notifiable) *database.Message {
	return database.NewMessage().
		SetType("order.placed").
		SetTitle("Order received").
		SetBody("We got your order " + o.OrderNumber).
		AddData("order_number", o.OrderNumber).
		AddData("total", o.Total)
}

func (o OrderPlacedNotification) Format(channel string, _ notification.Notifiable) any {
	return map[string]any{"kind": "order_placed", "order_number": o.OrderNumber, "total": o.Total}
}

// --- custom test channel ---

// TestChannel proves pluggability without real credentials: it only logs. It
// deliberately does NOT implement notification.Formatter, so the notifier falls
// back to the notification's ChannelFormatter.Format("test", ...).
type TestChannel struct {
	failNext bool // when true, the next Send fails (used by the OnError demo)
}

func (*TestChannel) Name() string { return "test" }

func (t *TestChannel) Send(ctx context.Context, n notification.Notifiable, message any) error {
	log.Printf("[notify:test] -> user=%s payload=%v", n.GetID(), message)
	return nil
}
