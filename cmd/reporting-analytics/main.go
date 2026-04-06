// reporting-analytics service entrypoint.
// Registers the vertical interceptor and reporting service handlers.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/reporting"
)

func main() {
	// In production, the Loader would be wired with Redis + DB.
	// For now, we pass nil (vertical interceptor skipped) to allow startup.
	srv := server.New(server.Config{
		Name:        "reporting-analytics",
		Port:        8085,
		MetricsPort: 9095,
		// Loader: vertical.NewLoader(redisClient, dbStore, nil),
	}, nil)

	// Create service with nil repo (to be wired with real DB)
	_ = reporting.NewHandler(reporting.NewService(nil))

	// Register gRPC service here when proto generation is set up:
	// reportingpb.RegisterReportingServiceServer(srv.GRPCServer(), handler)

	log.Printf("reporting-analytics service ready")
	if err := srv.Run(); err != nil {
		log.Fatalf("reporting-analytics: %v", err)
	}
}
