// billing service entrypoint.
// Universal service — no vertical interceptor. Billing is tenant-scoped via
// explicit tenant_id in every request, not via the HTTP/gRPC request context.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	billingv1 "github.com/wegofwd2020/thittam/gen/billing/v1"
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/iamclient"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
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

	// --- NATS JetStream (optional) ---
	// Billing must still boot without NATS configured (e.g. local/dev). The
	// outbox write in SuspendSubscription always happens regardless; only the
	// relay (below), which drains the outbox to NATS, is gated on pub != nil.
	var pub *jetstream.Publisher
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		nc, err := nats.Connect(natsURL,
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)
		if err != nil {
			log.Fatalf("billing: startup: connect to NATS: %v", err)
		}
		defer func() { _ = nc.Drain() }()

		js, err := nc.JetStream()
		if err != nil {
			log.Fatalf("billing: startup: create JetStream context: %v", err)
		}
		pub = jetstream.NewPublisher(js)
	} else {
		log.Printf("billing: NATS_URL not set — subscription.suspended will not be published")
	}

	// --- Repository and service ---
	// No Redis or vertical loader: billing is universal and does not
	// look up per-tenant vertical config on the request path.
	repo := billingdb.NewPostgres(pool)
	svc := billing.NewService(repo)

	// --- Outbox relay (#126) ---
	// Drains event_outbox → NATS in-process. Only runs when NATS is configured
	// (same gate as the publisher); reuses the pool + publisher.
	if pub != nil {
		relayCtx, cancelRelay := context.WithCancel(ctx)
		defer cancelRelay()
		relay := billing.NewRelay(repo, pub)
		go relay.Run(relayCtx)
		log.Printf("billing: outbox relay started")
	}

	// --- Document service client (for invoice PDF download URLs) ---
	// DOCUMENT_SERVICE_ADDR is T3 (non-sensitive endpoint), safe as env var.
	// mTLS is handled by the Istio sidecar; we connect with plain credentials
	// inside the mesh and let Envoy handle TLS termination.
	var docClient documentv1.DocumentServiceClient
	if addr := os.Getenv("DOCUMENT_SERVICE_ADDR"); addr != "" {
		// ForwardAuthUnaryClientInterceptor forwards the caller's bearer token
		// so document verifies the same token and authorizes the same caller
		// (#138). Without it, document's fail-closed auth interceptor rejects
		// this call as Unauthenticated.
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(interceptor.ForwardAuthUnaryClientInterceptor()),
		)
		if err != nil {
			log.Fatalf("billing: startup: connect to document service: %v", err)
		}
		defer func() { _ = conn.Close() }()
		docClient = documentv1.NewDocumentServiceClient(conn)
	} else {
		log.Printf("billing: DOCUMENT_SERVICE_ADDR not set — DownloadInvoice will return Unavailable")
	}

	// --- IAM permission checker ---
	// billing cannot authorize without IAM. #138/#149's convention for the JWT
	// public key applies here too: a service that cannot enforce its
	// guarantees does not start.
	//
	// iamPerm is the concrete *iamclient.PermissionChecker here, so this is a
	// plain pointer comparison. Assigning it to an interface field first would
	// produce a non-nil interface wrapping a nil pointer, and the check would
	// never fire.
	iamPerm, closeIAM, err := iamclient.DialFromEnv("billing")
	if err != nil {
		log.Fatalf("billing: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()
	if iamPerm == nil {
		log.Fatalf("billing: startup: %s is not set; billing cannot authorize without a permission checker", iamclient.EnvAddr)
	}

	handler := billing.NewHandlerWithDeps(svc, iamPerm, docClient)

	// --- gRPC server ---
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("billing: startup: load JWT public key: %v", err)
	}
	srv := server.New(server.Config{
		Name:        "billing",
		Port:        8089,
		MetricsPort: 9099,
		// Loader: nil — vertical interceptor is not used for this service.
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
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
