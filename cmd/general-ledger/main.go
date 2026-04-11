// general-ledger service entrypoint.
// Implements double-entry accounting: chart of accounts, journal entries,
// trial balance, and period closing. Source of truth for all financials.
//
// The general-ledger is a universal service — no vertical interceptor is loaded.
// Tenant IDs are provided explicitly in every request, enabling internal
// service-to-service calls (IAM seeder, reporting-analytics) without HTTP context.
package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/ledger"
	ledgerdb "github.com/wegofwd2020/thittam/services/ledger/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("general-ledger: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("general-ledger: startup: ping database: %v", err)
	}

	// --- Repository and service ---
	// No Redis or vertical loader: the ledger is universal and does not
	// look up per-tenant vertical config on the request path.
	repo := ledgerdb.NewPostgres(pool)
	svc := ledger.NewService(repo)
	handler := ledger.NewHandler(svc)

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "general-ledger",
		Port:        8083,
		MetricsPort: 9093,
		// Loader: nil — vertical interceptor is not used for this service.
	}, nil)

	ledgerv1.RegisterLedgerServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})

	log.Printf("general-ledger service ready on :8083")
	if err := srv.Run(); err != nil {
		log.Fatalf("general-ledger: %v", err)
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
		log.Fatalf("general-ledger: startup: required environment variable %s is not set", key)
	}
	return v
}
