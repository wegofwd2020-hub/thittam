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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	budgetv1 "github.com/wegofwd2020/thittam/gen/budget/v1"
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

	// --- Repository and service ---
	repo := budgetdb.NewPostgres(pool)
	svc := budget.NewService(repo)
	handler := budget.NewHandler(svc)

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "budget-planning",
		Port:        8081,
		MetricsPort: 9091,
		Loader:      loader,
	}, nil)

	budgetv1.RegisterBudgetServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})

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

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("budget-planning: startup: required environment variable %s is not set", key)
	}
	return v
}
