// reporting-analytics service entrypoint.
// Registers the vertical interceptor and reporting service handlers.
//
// The vertical interceptor injects the tenant's vertical config into the gRPC
// context on every request, enabling report-definition validation and
// entity-label enrichment without database round-trips.
//
// NATS JetStream consumers maintain the read-model projection tables. Six
// domain event types are handled by ProjectionConsumer. Each subject gets its
// own durable push consumer so the service can resume from where it left off
// after a restart without replaying the entire stream.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	reportingv1 "github.com/wegofwd2020/thittam/gen/reporting/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
	"github.com/wegofwd2020/thittam/services/reporting"
	reportingdb "github.com/wegofwd2020/thittam/services/reporting/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("reporting-analytics: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("reporting-analytics: startup: ping database: %v", err)
	}

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer func() { _ = rdb.Close() }()

	// --- NATS JetStream ---
	natsURL := requireenv("NATS_URL")
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("reporting-analytics: startup: connect to NATS: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("reporting-analytics: startup: create JetStream context: %v", err)
	}

	// --- Vertical config loader ---
	vdb := verticaldb.NewStore(pool)
	loader := vertical.NewLoader(rdb, vdb, nil)

	// --- Repository and service ---
	repo := reportingdb.NewPostgres(pool)
	svc := reporting.NewService(repo)
	handler := reporting.NewHandler(svc)

	// --- Projection consumer ---
	// ProjectionConsumer maintains six read-model tables by consuming domain
	// events from NATS JetStream. Each subject gets a durable push consumer
	// named "reporting-<subject>" so delivery resumes after restarts.
	sub := &natsSubscriber{js: js}
	consumer := reporting.NewProjectionConsumer(repo, repo, slogLogger{})
	if err := consumer.Register(sub); err != nil {
		log.Fatalf("reporting-analytics: startup: register projection consumer: %v", err)
	}
	defer sub.drain()

	// --- gRPC server ---
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("reporting-analytics: startup: load JWT public key: %v", err)
	}
	srv := server.New(server.Config{
		Name:                    "reporting-analytics",
		Port:                    8085,
		MetricsPort:             9095,
		Loader:                  loader,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
	}, nil)

	reportingv1.RegisterReportingServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})
	srv.RegisterHealthChecker("projection-lag", reporting.NewLagChecker(repo))

	log.Printf("reporting-analytics service ready on :8085")
	if err := srv.Run(); err != nil {
		log.Fatalf("reporting-analytics: %v", err)
	}
}

// ── natsSubscriber ────────────────────────────────────────────────────────────

// natsSubscriber bridges reporting.Subscriber to NATS JetStream.
// Each Subscribe call creates a durable push consumer for the given subject.
// Durable names are stable across restarts: "reporting-<subject-with-dashes>".
type natsSubscriber struct {
	js   nats.JetStreamContext
	subs []*nats.Subscription
}

func (s *natsSubscriber) Subscribe(subject string, handler func(ctx context.Context, msg reporting.Message) error) error {
	// Derive a stable durable name from the subject so delivery state persists
	// across pod restarts. E.g. "thittam.expense.approved" → "reporting-thittam-expense-approved".
	durable := "reporting-" + strings.ReplaceAll(subject, ".", "-")

	sub, err := s.js.Subscribe(subject, func(m *nats.Msg) {
		// ProjectionConsumer handlers call msg.Ack() or msg.Nak() themselves
		// based on whether the write succeeded. We ignore the return value here
		// because a failed Ack/Nak will time out and be redelivered by NATS.
		if err := handler(context.Background(), &natsMessage{m}); err != nil {
			slog.Error("reporting: projection handler error",
				"subject", subject,
				"error", err,
			)
		}
	},
		nats.Durable(durable),
		nats.ManualAck(),
		nats.AckExplicit(),
	)
	if err != nil {
		return fmt.Errorf("reporting: nats subscribe %s: %w", subject, err)
	}
	s.subs = append(s.subs, sub)
	slog.Info("reporting: projection consumer subscribed", "subject", subject, "durable", durable)
	return nil
}

func (s *natsSubscriber) drain() {
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
}

// natsMessage adapts *nats.Msg to the reporting.Message interface.
type natsMessage struct{ m *nats.Msg }

func (n *natsMessage) Subject() string { return n.m.Subject }
func (n *natsMessage) Data() []byte    { return n.m.Data }
func (n *natsMessage) Ack() error      { return n.m.Ack() }
func (n *natsMessage) Nak() error      { return n.m.Nak() }

// ── health checkers ───────────────────────────────────────────────────────────

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

// redisChecker implements observability.HealthChecker for the Redis connection.
// /readyz returns 503 when Redis is unreachable — the vertical config cache
// cannot be populated, which would cause all tenant-context lookups to fail.
type redisChecker struct{ rdb *redis.Client }

func (c *redisChecker) CheckHealth(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// ── slogLogger ────────────────────────────────────────────────────────────────

// slogLogger bridges reporting.Logger to the stdlib slog package.
type slogLogger struct{}

func (slogLogger) Info(msg string, kvs ...interface{})  { slog.Info(msg, kvs...) }
func (slogLogger) Warn(msg string, kvs ...interface{})  { slog.Warn(msg, kvs...) }
func (slogLogger) Error(msg string, kvs ...interface{}) { slog.Error(msg, kvs...) }

// ── requireenv ────────────────────────────────────────────────────────────────

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("reporting-analytics: startup: required environment variable %s is not set", key)
	}
	return v
}

