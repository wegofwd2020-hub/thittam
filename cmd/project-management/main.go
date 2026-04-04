// project-management service entrypoint.
// Registers the vertical interceptor and project service handlers.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/project"
)

func main() {
	// In production, the Loader would be wired with Redis + DB.
	// For now, we pass nil (vertical interceptor skipped) to allow startup.
	srv := server.New(server.Config{
		Name: "project-management",
		Port: 8080,
		// Loader: vertical.NewLoader(redisClient, dbStore, nil),
	}, nil)

	// Create service with nil repo (to be wired with real DB)
	_ = project.NewHandler(project.NewService(nil))

	// Register gRPC service here when proto generation is set up:
	// projectpb.RegisterProjectServiceServer(srv.GRPCServer(), handler)

	log.Printf("project-management service ready")
	if err := srv.Run(); err != nil {
		log.Fatalf("project-management: %v", err)
	}
}
