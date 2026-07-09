// expense-tracking service entrypoint.
// Registers the vertical interceptor and expense service handlers.
//
// The vertical interceptor injects the tenant's vertical config into the gRPC
// context on every request, enabling category validation, PO enforcement,
// and approval-workflow limits without database round-trips.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	expensev1 "github.com/wegofwd2020/thittam/gen/expense/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/iamclient"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
	"github.com/wegofwd2020/thittam/services/expense"
	expensedb "github.com/wegofwd2020/thittam/services/expense/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("expense-tracking: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("expense-tracking: startup: ping database: %v", err)
	}

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer func() { _ = rdb.Close() }()

	// --- Vertical config loader ---
	vdb := verticaldb.NewStore(pool)
	loader := vertical.NewLoader(rdb, vdb, nil)

	// --- NATS JetStream ---
	natsURL := requireenv("NATS_URL")
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("expense-tracking: startup: connect to NATS: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("expense-tracking: startup: create JetStream context: %v", err)
	}
	pub := jetstream.NewPublisher(js)

	// --- IAM permission checker (ADR-014 Phase 2) ---
	iamPerm, closeIAM, err := iamclient.DialFromEnv("expense-tracking")
	if err != nil {
		log.Fatalf("expense-tracking: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()

	// --- Repository and service ---
	repo := expensedb.NewPostgres(pool)
	svc := expense.NewService(repo, &expensePublisher{pub: pub})
	handler := expense.NewHandler(svc)
	if iamPerm != nil {
		handler = handler.WithPermissionChecker(iamPerm)
	}

	// --- gRPC server ---
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("expense-tracking: startup: load JWT public key: %v", err)
	}
	srv := server.New(server.Config{
		Name:        "expense-tracking",
		Port:        8082,
		MetricsPort: 9092,
		Loader:      loader,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
	}, nil)

	expensev1.RegisterExpenseServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})

	log.Printf("expense-tracking service ready on :8082")
	if err := srv.Run(); err != nil {
		log.Fatalf("expense-tracking: %v", err)
	}
}

// dbChecker implements observability.HealthChecker for the PostgreSQL pool.
type dbChecker struct{ pool *pgxpool.Pool }

func (c *dbChecker) CheckHealth(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// natsChecker implements observability.HealthChecker for the NATS connection.
// /readyz returns 503 when NATS is unreachable so Kubernetes stops routing
// traffic to a pod that cannot publish domain events.
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

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("expense-tracking: startup: required environment variable %s is not set", key)
	}
	return v
}

// expensePublisher adapts *jetstream.Publisher to expense.EventPublisher.
// Lives in cmd/ (composition root) to avoid import cycles between
// pkg/jetstream and services/expense.
type expensePublisher struct{ pub *jetstream.Publisher }

func (p *expensePublisher) PublishExpenseSubmitted(ctx context.Context, e *expense.Expense) error {
	return p.pub.Publish(ctx, events.SubjectExpenseSubmitted, e.TenantID, events.ExpenseSubmittedPayload{
		ExpenseID:    e.ID,
		ProductionID: e.ProductionID,
		CategoryID:   e.CategoryID,
		Amount:       e.Amount.StringFixed(2),
		SubmittedBy:  e.SubmittedBy.String(),
		SubmittedAt:  e.CreatedAt,
		Description:  e.Description,
	})
}

func (p *expensePublisher) PublishExpenseApproved(ctx context.Context, e *expense.Expense) error {
	approvedAt := e.CreatedAt
	if e.ApprovedAt != nil {
		approvedAt = *e.ApprovedAt
	}
	return p.pub.Publish(ctx, events.SubjectExpenseApproved, e.TenantID, events.ExpenseApprovedPayload{
		ExpenseID:    e.ID,
		ProductionID: e.ProductionID,
		CategoryID:   e.CategoryID,
		Amount:       e.Amount.StringFixed(2),
		ApprovedAt:   approvedAt,
		Month:        approvedAt.Format("2006-01"),
	})
}
