// notifications service entrypoint.
// Delivers transactional messages via email (SendGrid), SMS (Twilio),
// in-app (WebSocket/SSE), and push (Firebase FCM).
// Triggered by NATS events from all other services and the thin gRPC
// Send/Dispatch RPCs for synchronous sends.
//
// Channel senders are wired from environment variables at startup.
// Provider credentials follow Rule #2 — T2 secrets loaded from env vars
// injected by Kubernetes Secrets; never hardcoded.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	nats "github.com/nats-io/nats.go"
	notificationsv1 "github.com/wegofwd2020/thittam/gen/notifications/v1"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/notifications"
	notificationsdb "github.com/wegofwd2020/thittam/services/notifications/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("notifications: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("notifications: startup: ping database: %v", err)
	}

	// --- NATS JetStream ---
	// NATS_URL is T3 (service endpoint) — read from env var directly.
	natsURL := requireenv("NATS_URL")
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("notifications: startup: connect to NATS: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("notifications: startup: create JetStream context: %v", err)
	}

	// --- Channel senders ---
	// Each sender implementation wraps an external provider. Credentials are
	// injected from Kubernetes Secrets as environment variables (Rule #2, T2).
	// A nil entry means that channel is not configured — attempts return
	// ErrNoSenderForChannel rather than panicking.
	senders := map[string]notifications.ChannelSender{
		// "email": sendgrid.NewSender(requireenv("SENDGRID_API_KEY")),
		// "sms":   twilio.NewSender(requireenv("TWILIO_ACCOUNT_SID"), requireenv("TWILIO_AUTH_TOKEN")),
		// "push":  fcm.NewSender(requireenv("FCM_SERVER_KEY")),
		// "in_app": sse.NewSender(hub),
	}

	// --- Repository and service ---
	repo := notificationsdb.NewPostgres(pool)
	svc := notifications.NewService(repo, senders)
	handler := notifications.NewHandler(svc)

	// --- JetStream consumers ---
	// Two durable consumers provide at-least-once delivery with DLQ fallback:
	//   notifications-financial — financial stream (budget, expense) with 5-attempt backoff
	//   notifications-events    — all-events stream (project, document, etc.)
	// Both consumers must already exist in NATS (created via `make nats-provision`).
	d := &dispatcher{svc: svc}

	subFinancial, err := jetstream.Subscribe(js,
		jetstream.StreamFinancial,
		jetstream.ConsumerNotificationsFinancial,
		d.dispatch,
	)
	if err != nil {
		log.Fatalf("notifications: startup: subscribe to financial stream: %v", err)
	}
	defer func() { _ = subFinancial.Unsubscribe() }()

	subEvents, err := jetstream.Subscribe(js,
		jetstream.StreamEvents,
		jetstream.ConsumerNotificationsEvents,
		d.dispatch,
	)
	if err != nil {
		log.Fatalf("notifications: startup: subscribe to events stream: %v", err)
	}
	defer func() { _ = subEvents.Unsubscribe() }()

	// --- gRPC server ---
	// Notifications is a universal service — no vertical interceptor needed.
	srv := server.New(server.Config{
		Name:        "notifications",
		Port:        8087,
		MetricsPort: 9097,
	}, nil)

	notificationsv1.RegisterNotificationsServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})

	log.Printf("notifications service ready on :8087")
	if err := srv.Run(); err != nil {
		log.Fatalf("notifications: %v", err)
	}
}

// dbChecker implements observability.HealthChecker for the PostgreSQL pool.
type dbChecker struct{ pool *pgxpool.Pool }

func (c *dbChecker) CheckHealth(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// natsChecker implements observability.HealthChecker for the NATS connection.
type natsChecker struct{ nc *nats.Conn }

func (c *natsChecker) CheckHealth(_ context.Context) error {
	if !c.nc.IsConnected() {
		return nats.ErrConnectionClosed
	}
	return nil
}

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("notifications: startup: required environment variable %s is not set", key)
	}
	return v
}
