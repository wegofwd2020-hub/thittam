// budget-planning service entrypoint.
// Registers the vertical interceptor and budget service handlers.
//
// The vertical interceptor injects the tenant's vertical config into the gRPC
// context on every request, allowing the service to validate budget categories,
// resolve default account codes, and populate templates without DB round-trips.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	budgetv1 "github.com/wegofwd2020/thittam/gen/budget/v1"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/iamclient"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
	"github.com/wegofwd2020/thittam/services/budget"
	budgetdb "github.com/wegofwd2020/thittam/services/budget/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("budget-planning: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("budget-planning: startup: ping database: %v", err)
	}

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer rdb.Close()

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
		log.Fatalf("budget-planning: startup: connect to NATS: %v", err)
	}
	defer nc.Drain()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("budget-planning: startup: create JetStream context: %v", err)
	}
	pub := jetstream.NewPublisher(js)

	// --- IAM permission checker (ADR-014 Phase 2) ---
	// Optional: when IAM_SERVICE_ADDR is unset, RequirePermission calls in
	// the handler are no-ops. This is the rollback-safe default.
	iamPerm, closeIAM, err := iamclient.DialFromEnv("budget-planning")
	if err != nil {
		log.Fatalf("budget-planning: startup: dial IAM: %v", err)
	}
	defer closeIAM()

	// --- Repository and service ---
	repo := budgetdb.NewPostgres(pool)
	svc := budget.NewService(repo, &budgetPublisher{pub: pub})
	handler := budget.NewHandler(svc)
	if iamPerm != nil {
		handler = handler.WithPermissionChecker(iamPerm)
	}

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "budget-planning",
		Port:        8081,
		MetricsPort: 9091,
		Loader:      loader,
	}, nil)

	budgetv1.RegisterBudgetServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})

	log.Printf("budget-planning service ready on :8081")
	if err := srv.Run(); err != nil {
		log.Fatalf("budget-planning: %v", err)
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
		log.Fatalf("budget-planning: startup: required environment variable %s is not set", key)
	}
	return v
}

// budgetPublisher adapts *jetstream.Publisher to budget.EventPublisher.
type budgetPublisher struct{ pub *jetstream.Publisher }

func (p *budgetPublisher) PublishBudgetApproved(ctx context.Context, b *budget.Budget) error {
	return p.pub.Publish(ctx, events.SubjectBudgetApproved, b.TenantID, events.BudgetApprovedPayload{
		BudgetID:      b.ID,
		ProductionID:  b.ProductionID,
		TotalBudgeted: b.TotalAmount.StringFixed(2),
	})
}
