// Command notify-demo exercises go-notification end-to-end against live
// Postgres: database (in-app) channel + custom test channel fan-out, async
// worker pool + thread-safety, Sync() blocking, Via() override, OnError after
// retries, rate limiting, and Close() draining. Fase 5 verification harness.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gostore/internal/bootstrap"
	"gostore/internal/db"
	"gostore/internal/dto"
	"gostore/internal/models"
	"gostore/internal/notify"

	notification "github.com/gopackx/go-notification"
)

var checks []string

func check(name string, ok bool, detail string) {
	mark := "PASS"
	if !ok {
		mark = "FAIL"
	}
	line := fmt.Sprintf("[%s] %s — %s", mark, name, detail)
	checks = append(checks, line)
	fmt.Println(line)
}

func main() {
	conn, err := db.Open()
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer conn.Close()
	if err := bootstrap.Fresh(conn.SQL); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()

	svc, err := notify.Setup(ctx, conn.SQL)
	if err != nil {
		log.Fatalf("notify setup: %v", err)
	}
	defer svc.Close()

	user := models.User{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "Notify User",
		Email: "notify@gostore.dev", Password: "x", Role: "customer", Active: true}
	if err := conn.Gorm.Create(&user).Error; err != nil {
		log.Fatalf("create user: %v", err)
	}

	// --- 1. Async fan-out: Welcome -> database + test ---
	svc.Welcome(ctx, user)
	waitFor(func() bool { n, _ := svc.Unread(ctx, user); return len(n) >= 1 }, 2*time.Second)
	unread, _ := svc.Unread(ctx, user)
	check("Welcome persisted via database channel (async)", len(unread) == 1 && unread[0].Type == "user.welcome",
		fmt.Sprintf("%d unread, type=%s", len(unread), typeOf(unread)))

	// --- 2. OrderPlaced fan-out ---
	svc.OrderPlaced(ctx, user, models.Order{OrderNumber: "ORD-X", Total: 42})
	waitFor(func() bool { n, _ := svc.Unread(ctx, user); return len(n) >= 2 }, 2*time.Second)
	unread, _ = svc.Unread(ctx, user)
	check("OrderPlaced persisted (2 unread total)", len(unread) == 2, fmt.Sprintf("%d unread", len(unread)))

	// --- 3. MarkAllRead ---
	_ = svc.MarkAllRead(ctx, user)
	unread, _ = svc.Unread(ctx, user)
	check("MarkAllRead clears unread", len(unread) == 0, fmt.Sprintf("%d unread after mark", len(unread)))

	// --- 4. Thread-safety: 50 concurrent async sends all persist ---
	const N = 50
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); svc.Welcome(ctx, user) }()
	}
	wg.Wait()
	waitFor(func() bool { n, _ := svc.Unread(ctx, user); return len(n) >= N }, 5*time.Second)
	unread, _ = svc.Unread(ctx, user)
	check("Thread-safe async pool: 50 concurrent sends persist", len(unread) == N, fmt.Sprintf("%d/%d persisted", len(unread), N))

	// --- raw notifier primitives (Sync / Via / OnError / Close / rate limit) ---
	rawTests()

	fmt.Println("\n================ Fase 5 coverage ================")
	fail := 0
	for _, c := range checks {
		if c[1:5] == "FAIL" {
			fail++
		}
	}
	fmt.Printf("Total: %d checks, %d failed\n", len(checks), fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// --- raw notifier tests using in-memory counting channels ---

type demoUser struct{ id string }

func (u demoUser) GetID() string                      { return u.id }
func (u demoUser) RouteNotificationFor(string) string { return "" }

// pingNotif fans out to channels declared in Via; Format feeds custom channels.
type pingNotif struct{ via []string }

func (p pingNotif) Via(notification.Notifiable) []string { return p.via }
func (p pingNotif) Format(channel string, _ notification.Notifiable) any {
	return map[string]any{"channel": channel}
}

type countingChannel struct {
	name  string
	count int64
	fail  bool
}

func (c *countingChannel) Name() string { return c.name }
func (c *countingChannel) Send(ctx context.Context, n notification.Notifiable, msg any) error {
	if c.fail {
		return errors.New("simulated permanent failure")
	}
	atomic.AddInt64(&c.count, 1)
	return nil
}

func rawTests() {
	u := demoUser{id: "raw-1"}

	// Sync(): blocks until delivered -> count is 1 immediately after Send.
	sc := &countingChannel{name: "sync"}
	n := notification.New(notification.Config{}) // async by default
	n.RegisterChannel(sc)
	_ = n.Send(context.Background(), u, pingNotif{via: []string{"sync"}}, notification.Sync())
	check("Sync() blocks until delivered", atomic.LoadInt64(&sc.count) == 1,
		fmt.Sprintf("count=%d immediately after Send", atomic.LoadInt64(&sc.count)))
	n.Close()

	// Via() override: notification.Via([a,b]) but override to only "a".
	a := &countingChannel{name: "a"}
	b := &countingChannel{name: "b"}
	n2 := notification.New(notification.Config{})
	n2.RegisterChannel(a)
	n2.RegisterChannel(b)
	_ = n2.Send(context.Background(), u, pingNotif{via: []string{"a", "b"}}, notification.Sync(), notification.Via("a"))
	check("Via() override sends only the chosen channel", atomic.LoadInt64(&a.count) == 1 && atomic.LoadInt64(&b.count) == 0,
		fmt.Sprintf("a=%d b=%d", a.count, b.count))
	n2.Close()

	// OnError: a failing channel triggers OnError after retries are exhausted.
	var onErrCalled int64
	var lastErr error
	n3 := notification.New(notification.Config{
		MaxRetries: 2, RetryDelay: 5 * time.Millisecond,
		OnError: func(ctx context.Context, nt notification.Notifiable, ch string, err error) {
			atomic.AddInt64(&onErrCalled, 1)
			lastErr = err
		},
	})
	n3.RegisterChannel(&countingChannel{name: "boom", fail: true})
	_ = n3.Send(context.Background(), u, pingNotif{via: []string{"boom"}}, notification.Sync())
	check("OnError fires after retries exhausted", atomic.LoadInt64(&onErrCalled) == 1,
		fmt.Sprintf("OnError called %d time(s), err=%v", onErrCalled, lastErr))
	n3.Close()

	// Close() drains and subsequent Send returns ErrClosed.
	n4 := notification.New(notification.Config{})
	n4.RegisterChannel(&countingChannel{name: "x"})
	n4.Close()
	err := n4.Send(context.Background(), u, pingNotif{via: []string{"x"}})
	check("Send after Close returns ErrClosed", errors.Is(err, notification.ErrClosed), fmt.Sprintf("err=%v", err))

	// Rate limit: tight bucket, fire 20 sync; all handled without crashing.
	rl := &countingChannel{name: "rl"}
	n5 := notification.New(notification.Config{MaxRetries: 0})
	n5.RegisterChannel(rl)
	n5.SetRateLimit("rl", 5, 100*time.Millisecond, 5)
	handled := 0
	for i := 0; i < 20; i++ {
		if err := n5.Send(context.Background(), u, pingNotif{via: []string{"rl"}}, notification.Sync()); err == nil {
			handled++
		}
	}
	check("Rate limiter handles burst without crashing", handled >= 5,
		fmt.Sprintf("%d/20 sent ok, delivered=%d", handled, atomic.LoadInt64(&rl.count)))
	n5.Close()
}

func waitFor(cond func() bool, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func typeOf(rows []dto.NotificationView) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0].Type
}
