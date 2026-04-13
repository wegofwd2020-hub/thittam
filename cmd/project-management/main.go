// project-management service entrypoint.
// Registers the vertical interceptor and project service handlers.
//
// The vertical interceptor injects the tenant's vertical config into the gRPC
// context on every request, allowing service methods to validate phase types
// and return industry-specific labels without database round-trips (Redis cache).
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

	projectv1 "github.com/wegofwd2020/thittam/gen/project/v1"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/iamclient"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
	"github.com/wegofwd2020/thittam/services/project"
	projectdb "github.com/wegofwd2020/thittam/services/project/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("project-management: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("project-management: startup: ping database: %v", err)
	}

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer func() { _ = rdb.Close() }()

	// --- Vertical config loader ---
	// The loader provides a Redis-cached (5-min TTL) view of the tenant's
	// vertical configuration. It injects the config into the gRPC context
	// via the server's UnaryInterceptor / StreamInterceptor.
	vdb := verticaldb.NewStore(pool)
	loader := vertical.NewLoader(rdb, vdb, nil) // nil → uses default logger

	// --- NATS JetStream ---
	natsURL := requireenv("NATS_URL")
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("project-management: startup: connect to NATS: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("project-management: startup: create JetStream context: %v", err)
	}
	pub := jetstream.NewPublisher(js)

	// --- IAM permission checker (ADR-014 Phase 2) ---
	iamPerm, closeIAM, err := iamclient.DialFromEnv("project-management")
	if err != nil {
		log.Fatalf("project-management: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()

	// --- Repository and service ---
	repo := projectdb.NewPostgres(pool)
	svc := project.NewService(repo, &projectPublisher{pub: pub})
	handler := project.NewHandler(svc)
	if iamPerm != nil {
		handler = handler.WithPermissionChecker(iamPerm)
	}

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "project-management",
		Port:        8080,
		MetricsPort: 9090,
		Loader:      loader,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryCallerInterceptor()},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamCallerInterceptor()},
	}, nil)

	projectv1.RegisterProjectServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})

	log.Printf("project-management service ready on :8080")
	if err := srv.Run(); err != nil {
		log.Fatalf("project-management: %v", err)
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
		log.Fatalf("project-management: startup: required environment variable %s is not set", key)
	}
	return v
}

// projectPublisher adapts *jetstream.Publisher to project.EventPublisher.
type projectPublisher struct{ pub *jetstream.Publisher }

func (p *projectPublisher) PublishProductionCreated(ctx context.Context, prod *project.Production) error {
	return p.pub.Publish(ctx, events.SubjectProjectCreated, prod.TenantID, events.ProjectCreatedPayload{
		ProjectID: prod.ID,
		Title:     prod.Title,
		Status:    prod.Status,
		StartDate: prod.StartDate,
	})
}

func (p *projectPublisher) PublishCrewMemberAdded(ctx context.Context, c *project.CrewMember) error {
	return p.pub.Publish(ctx, events.SubjectProjectMemberAssigned, c.TenantID, events.ProjectMemberAssignedPayload{
		ProjectID: c.ProductionID,
		// MemberCount requires a separate query; publish with 0 for now — consumers
		// should treat this as a "member added" signal and re-query if needed.
		MemberCount: 0,
	})
}
