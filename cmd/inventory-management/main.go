// inventory-management service entrypoint.
// Registers the vertical interceptor and inventory service handlers.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/inventory"
)

func main() {
	// In production, the Loader would be wired with Redis + DB.
	// For now, we pass nil (vertical interceptor skipped) to allow startup.
	srv := server.New(server.Config{
		Name: "inventory-management",
		Port: 8084,
		// Loader: vertical.NewLoader(redisClient, dbStore, nil),
	}, nil)

	// Create service with nil repo (to be wired with real DB)
	_ = inventory.NewHandler(inventory.NewService(nil))

	// Register gRPC service here when proto generation is set up:
	// inventorypb.RegisterInventoryServiceServer(srv.GRPCServer(), handler)

	log.Printf("inventory-management service ready")
	if err := srv.Run(); err != nil {
		log.Fatalf("inventory-management: %v", err)
	}
}
