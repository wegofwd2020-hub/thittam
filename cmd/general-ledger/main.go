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
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"

	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/iamclient"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
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

	// --- NATS JetStream ---
	// Financial events (journal.posted) go to the FINANCIAL stream with DLQ
	// protection (see pkg/jetstream/config.go and infra/nats/provision.sh).
	natsURL := requireenv("NATS_URL")
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("general-ledger: startup: connect to NATS: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("general-ledger: startup: create JetStream context: %v", err)
	}
	pub := jetstream.NewPublisher(js)

	// --- Repository and service ---
	// No Redis or vertical loader: the ledger is universal and does not
	// look up per-tenant vertical config on the request path.
	repo := ledgerdb.NewPostgres(pool)
	svc := ledger.NewService(repo, &ledgerPublisher{pub: pub})

	// --- IAM permission checker ---
	// The ledger cannot authorize without IAM. #138's convention for the JWT public
	// key applies here too: a service that cannot enforce its guarantees does not
	// start. Starting would serve codes.Internal on every accounting RPC, which reads
	// as a bug rather than as a misconfiguration.
	//
	// iamPerm is the concrete *iamclient.PermissionChecker here, so this is a plain
	// pointer comparison. Assigning it to an interface field first would produce a
	// non-nil interface wrapping a nil pointer, and the check would never fire.
	iamPerm, closeIAM, err := iamclient.DialFromEnv("general-ledger")
	if err != nil {
		log.Fatalf("general-ledger: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()
	if iamPerm == nil {
		log.Fatalf("general-ledger: startup: %s is not set; the ledger cannot authorize without a permission checker", iamclient.EnvAddr)
	}

	handler := ledger.NewHandler(svc, iamPerm)

	// --- gRPC server ---
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("general-ledger: startup: load JWT public key: %v", err)
	}
	srv := server.New(server.Config{
		Name:        "general-ledger",
		Port:        8083,
		MetricsPort: 9093,
		// Loader: nil — vertical interceptor is not used for this service.
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
	}, nil)

	ledgerv1.RegisterLedgerServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})

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

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("general-ledger: startup: required environment variable %s is not set", key)
	}
	return v
}

// ledgerPublisher adapts *jetstream.Publisher to ledger.EventPublisher.
type ledgerPublisher struct{ pub *jetstream.Publisher }

func (p *ledgerPublisher) PublishJournalPosted(ctx context.Context, je *ledger.JournalEntry) error {
	var prodID string
	if je.ProductionID != nil {
		prodID = je.ProductionID.String()
	}
	var postedAt time.Time
	if je.PostedAt != nil {
		postedAt = *je.PostedAt
	}

	// Compute totals from lines.
	var totalDebit, totalCredit float64
	for _, l := range je.Lines {
		totalDebit += l.DebitAmount.InexactFloat64()
		totalCredit += l.CreditAmount.InexactFloat64()
	}

	return p.pub.Publish(ctx, events.SubjectLedgerJournalPosted, je.TenantID, events.LedgerJournalPostedPayload{
		JournalEntryID: je.ID.String(),
		EntryNumber:    je.EntryNumber,
		ProductionID:   prodID,
		PeriodID:       je.PeriodID.String(),
		TotalDebit:     fmt.Sprintf("%.2f", totalDebit),
		TotalCredit:    fmt.Sprintf("%.2f", totalCredit),
		Narration:      je.Narration,
		PostedAt:       postedAt,
	})
}
