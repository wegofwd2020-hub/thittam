// document service entrypoint.
// Manages file storage, versioning, and access control via presigned URLs
// over an S3-compatible object store (MinIO in self-hosted; AWS S3 in cloud).
// Files are never routed through this service — clients upload and download
// directly to/from object storage.
//
// Object store credentials are T2 secrets injected from Kubernetes Secrets
// as environment variables — never hardcoded (Rule #2).
package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	// "github.com/nats-io/nats.go"  // uncomment when NATS publisher is wired
	"google.golang.org/grpc"

	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/document"
	documentdb "github.com/wegofwd2020/thittam/services/document/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("document: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("document: startup: ping database: %v", err)
	}

	// --- Object store ---
	// MinIO (self-hosted) / AWS S3 (cloud). Credentials are T2 secrets injected
	// as Kubernetes Secret environment variables.
	//
	// store = minio.NewStore(
	//     requireenv("MINIO_ENDPOINT"),
	//     requireenv("MINIO_ACCESS_KEY"),
	//     requireenv("MINIO_SECRET_KEY"),
	//     requireenv("MINIO_BUCKET"),
	// )
	//
	// A nil store causes the service to reject upload/download requests at
	// runtime rather than at startup — acceptable until the store stub lands.
	var store document.ObjectStore

	// --- Event publisher ---
	// NATS JetStream publisher for document.* events (uploaded, deleted, etc.).
	// When enabling, uncomment ALL of the following block together and add the
	// nats import above. Also uncomment the natsChecker registration below.
	//
	// natsURL := requireenv("NATS_URL")
	// nc, err := nats.Connect(natsURL)
	// if err != nil {
	//     log.Fatalf("document: startup: connect to NATS: %v", err)
	// }
	// defer nc.Drain()
	// js, err := nc.JetStream()
	// if err != nil {
	//     log.Fatalf("document: startup: JetStream context: %v", err)
	// }
	// publisher = natspublisher.New(js)
	var publisher document.EventPublisher
	// var nc *nats.Conn // set above when NATS is enabled

	// --- Repository, service, and handler ---
	repo := documentdb.NewPostgres(pool)
	svc := document.NewService(repo, store, publisher)
	handler := document.NewHandler(svc)

	// --- gRPC server ---
	// Document is a universal service — no vertical interceptor needed.
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("document: startup: load JWT public key: %v", err)
	}
	srv := server.New(server.Config{
		Name:                    "document",
		Port:                    8088,
		MetricsPort:             9098,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
	}, nil)

	documentv1.RegisterDocumentServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	// Uncomment when NATS publisher is wired (see event publisher block above):
	// srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})

	log.Printf("document service ready on :8088")
	if err := srv.Run(); err != nil {
		log.Fatalf("document: %v", err)
	}
}

// dbChecker implements observability.HealthChecker for the PostgreSQL pool.
type dbChecker struct{ pool *pgxpool.Pool }

func (c *dbChecker) CheckHealth(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// natsChecker implements observability.HealthChecker for the NATS connection.
// Uncomment the registration in main() when the NATS publisher is enabled.
//
// type natsChecker struct{ nc *nats.Conn }
// func (c *natsChecker) CheckHealth(_ context.Context) error {
//     if !c.nc.IsConnected() {
//         return errors.New("nats: not connected")
//     }
//     return nil
// }

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("document: startup: required environment variable %s is not set", key)
	}
	return v
}
