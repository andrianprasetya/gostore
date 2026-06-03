// Package notify wires go-notification into GoStore: a database (in-app)
// channel backed by the shared Postgres + a custom "test" channel (logs only,
// no real credentials). It exposes Welcome and OrderPlaced notifications that
// fan out to both channels.
package notify

import (
	"context"
	"database/sql"
	"log"

	"gostore/internal/dto"
	"gostore/internal/models"

	notification "github.com/gopackx/go-notification"
	"github.com/gopackx/go-notification/channel/database"
	"github.com/gopackx/go-notification/migrate"
)

// Service implements handlers.Notifier and handlers.NotificationCenter.
type Service struct {
	notifier *notification.Notifier
	dbCh     *database.Channel
}

// Setup migrates the notifications table, builds the notifier (async worker
// pool + retry + rate limit + OnError), and registers the database + test
// channels.
func Setup(ctx context.Context, sqlDB *sql.DB) (*Service, error) {
	if err := migrate.Up(ctx, sqlDB, database.DialectPostgreSQL, "notifications"); err != nil {
		return nil, err
	}

	dbCh := database.New(database.Config{
		Store: database.NewSQLStore(sqlDB, database.DialectPostgreSQL),
	})

	n := notification.New(notification.Config{
		WorkerPool: 10,
		MaxRetries: 3,
		OnError: func(ctx context.Context, nt notification.Notifiable, ch string, err error) {
			log.Printf("[notify] OnError channel=%s user=%s err=%v", ch, nt.GetID(), err)
		},
	})
	n.RegisterChannel(dbCh)
	n.RegisterChannel(&TestChannel{})
	n.SetRateLimit("test", 100, secondsToDur(1), 20)

	return &Service{notifier: n, dbCh: dbCh}, nil
}

// Notifier exposes the raw notifier (used by the Fase 5 demo for Sync/Via/Close).
func (s *Service) Notifier() *notification.Notifier { return s.notifier }

// Close drains the worker pool.
func (s *Service) Close() { s.notifier.Close() }

// --- handlers.Notifier ---

func (s *Service) Welcome(ctx context.Context, user models.User) {
	_ = s.notifier.Send(ctx, recipient{user}, WelcomeNotification{Name: user.Name})
}

func (s *Service) OrderPlaced(ctx context.Context, user models.User, order models.Order) {
	_ = s.notifier.Send(ctx, recipient{user}, OrderPlacedNotification{
		OrderNumber: order.OrderNumber, Total: order.Total,
	})
}

// --- handlers.NotificationCenter ---

func (s *Service) Unread(ctx context.Context, user models.User) ([]dto.NotificationView, error) {
	rows, err := s.dbCh.Unread(ctx, recipient{user}, 50)
	if err != nil {
		return nil, err
	}
	out := make([]dto.NotificationView, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.NotificationView{
			ID: r.ID, Type: r.Type, Title: r.Title, Body: r.Body,
			Data: r.Data, Read: r.ReadAt != nil, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) MarkAllRead(ctx context.Context, user models.User) error {
	return s.dbCh.MarkAllAsRead(ctx, recipient{user})
}

// recipient adapts a models.User to notification.Notifiable.
type recipient struct{ user models.User }

func (r recipient) GetID() string { return uitoa(r.user.ID) }
func (r recipient) RouteNotificationFor(channel string) string {
	// database + test channels don't need a route; mail would return email.
	if channel == "mail" {
		return r.user.Email
	}
	return ""
}
func (r recipient) GetNotifiableType() string { return "users" }

func uitoa(u uint64) string {
	if u == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for u > 0 {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
	}
	return string(b[i:])
}
