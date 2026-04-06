// document service entrypoint.
// Manages file storage, versioning, and access control via presigned URLs
// over an S3-compatible object store (MinIO in self-hosted; AWS S3 in cloud).
// Files are never routed through this service — clients upload and download
// directly to/from object storage.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/document"
)

func main() {
	srv := server.New(server.Config{
		Name:        "document",
		Port:        8088,
		MetricsPort: 9098,
	}, nil)

	// In production, wire dependencies from environment:
	//
	//   store     = minio.NewStore(
	//                 os.Getenv("MINIO_ENDPOINT"),
	//                 os.Getenv("MINIO_ACCESS_KEY"),   // from K8s Secret
	//                 os.Getenv("MINIO_SECRET_KEY"),   // from K8s Secret
	//                 os.Getenv("MINIO_BUCKET"),
	//               )
	//   publisher = nats.NewPublisher(natsConn)
	//   repo      = postgres.NewDocumentRepository(db)
	//
	// Provider credentials are NEVER hardcoded — injected from Kubernetes
	// Secrets as environment variables per Rule #2.
	_ = document.NewHandler(document.NewService(nil, nil, nil))

	// Register gRPC service here when proto generation is set up:
	// docpb.RegisterDocumentServiceServer(srv.GRPCServer(), handler)

	log.Printf("document service ready on :8088")
	if err := srv.Run(); err != nil {
		log.Fatalf("document: %v", err)
	}
}
