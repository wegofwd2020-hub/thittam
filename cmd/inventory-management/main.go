// inventory-management service entrypoint.
// Registers the vertical interceptor and inventory service handlers.
//
// The vertical interceptor injects the tenant's vertical config into the gRPC
// context on every request, enabling category validation and trackability rules
// without database round-trips.
package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	inventoryv1 "github.com/wegofwd2020/thittam/gen/inventory/v1"
	"github.com/wegofwd2020/thittam/pkg/iamclient"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
	"github.com/wegofwd2020/thittam/services/inventory"
	inventorydb "github.com/wegofwd2020/thittam/services/inventory/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("inventory-management: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("inventory-management: startup: ping database: %v", err)
	}

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer rdb.Close()

	// --- Vertical config loader ---
	vdb := verticaldb.NewStore(pool)
	loader := vertical.NewLoader(rdb, vdb, nil)

	// --- IAM permission checker (ADR-014 Phase 2) ---
	iamPerm, closeIAM, err := iamclient.DialFromEnv("inventory-management")
	if err != nil {
		log.Fatalf("inventory-management: startup: dial IAM: %v", err)
	}
	defer closeIAM()

	// --- Repository and service ---
	repo := inventorydb.NewPostgres(pool)
	svc := inventory.NewService(repo)
	handler := inventory.NewHandler(svc)
	if iamPerm != nil {
		handler = handler.WithPermissionChecker(iamPerm)
	}

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "inventory-management",
		Port:        8084,
		MetricsPort: 9094,
		Loader:      loader,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryCallerInterceptor()},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamCallerInterceptor()},
	}, nil)

	inventoryv1.RegisterInventoryServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})

	log.Printf("inventory-management service ready on :8084")
	if err := srv.Run(); err != nil {
		log.Fatalf("inventory-management: %v", err)
	}
}

// dbChecker implements observability.HealthChecker for the PostgreSQL pool.
type dbChecker struct{ pool *pgxpool.Pool }

func (c *dbChecker) CheckHealth(ctx context.Context) error {
	return c.pool.Ping(ctx)
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
		log.Fatalf("inventory-management: startup: required environment variable %s is not set", key)
	}
	return v
}
