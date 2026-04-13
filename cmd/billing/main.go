// billing service entrypoint.
// Universal service — no vertical interceptor. Billing is tenant-scoped via
// explicit tenant_id in every request, not via the HTTP/gRPC request context.
package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	billingv1 "github.com/wegofwd2020/thittam/gen/billing/v1"
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/billing"
	billingdb "github.com/wegofwd2020/thittam/services/billing/db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("billing: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("billing: startup: ping database: %v", err)
	}

	// --- Repository and service ---
	// No Redis or vertical loader: billing is universal and does not
	// look up per-tenant vertical config on the request path.
	// No NATS publisher: billing domain events are reserved for a future PR
	// when the billing saga (invoice.paid → subscription.activated) is wired up.
	repo := billingdb.NewPostgres(pool)
	svc := billing.NewService(repo)

	// --- Document service client (for invoice PDF download URLs) ---
	// DOCUMENT_SERVICE_ADDR is T3 (non-sensitive endpoint), safe as env var.
	// mTLS is handled by the Istio sidecar; we connect with plain credentials
	// inside the mesh and let Envoy handle TLS termination.
	var docClient documentv1.DocumentServiceClient
	if addr := os.Getenv("DOCUMENT_SERVICE_ADDR"); addr != "" {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("billing: startup: connect to document service: %v", err)
		}
		defer func() { _ = conn.Close() }()
		docClient = documentv1.NewDocumentServiceClient(conn)
	} else {
		log.Printf("billing: DOCUMENT_SERVICE_ADDR not set — DownloadInvoice will return Unavailable")
	}

	handler := billing.NewHandlerWithDeps(svc, docClient)

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "billing",
		Port:        8089,
		MetricsPort: 9099,
		// Loader: nil — vertical interceptor is not used for this service.
	}, nil)

	billingv1.RegisterBillingServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})

	log.Printf("billing service ready on :8089")
	if err := srv.Run(); err != nil {
		log.Fatalf("billing: %v", err)
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
		log.Fatalf("billing: startup: required environment variable %s is not set", key)
	}
	return v
}
