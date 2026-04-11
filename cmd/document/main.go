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
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
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
	// publisher = nats.NewPublisher(natsConn)
	var publisher document.EventPublisher

	// --- Repository, service, and handler ---
	repo := documentdb.NewPostgres(pool)
	svc := document.NewService(repo, store, publisher)
	handler := document.NewHandler(svc)

	// --- gRPC server ---
	// Document is a universal service — no vertical interceptor needed.
	srv := server.New(server.Config{
		Name:        "document",
		Port:        8088,
		MetricsPort: 9098,
	}, nil)

	documentv1.RegisterDocumentServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})

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

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("document: startup: required environment variable %s is not set", key)
	}
	return v
}
