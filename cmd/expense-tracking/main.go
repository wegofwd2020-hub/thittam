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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	expensev1 "github.com/wegofwd2020/thittam/gen/expense/v1"
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
	defer rdb.Close()

	// --- Vertical config loader ---
	vdb := verticaldb.NewStore(pool)
	loader := vertical.NewLoader(rdb, vdb, nil)

	// --- Repository and service ---
	repo := expensedb.NewPostgres(pool)
	svc := expense.NewService(repo)
	handler := expense.NewHandler(svc)

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "expense-tracking",
		Port:        8082,
		MetricsPort: 9092,
		Loader:      loader,
	}, nil)

	expensev1.RegisterExpenseServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})

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

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("expense-tracking: startup: required environment variable %s is not set", key)
	}
	return v
}
