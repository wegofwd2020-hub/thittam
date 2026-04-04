// budget-planning service entrypoint.
// Registers the vertical interceptor and budget service handlers.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/budget"
)

func main() {
	// In production, the Loader would be wired with Redis + DB.
	// For now, we pass nil (vertical interceptor skipped) to allow startup.
	srv := server.New(server.Config{
		Name: "budget-planning",
		Port: 8081,
		// Loader: vertical.NewLoader(redisClient, dbStore, nil),
	}, nil)

	// Create service with nil repo (to be wired with real DB)
	_ = budget.NewHandler(budget.NewService(nil))

	// Register gRPC service here when proto generation is set up:
	// budgetpb.RegisterBudgetServiceServer(srv.GRPCServer(), handler)

	log.Printf("budget-planning service ready")
	if err := srv.Run(); err != nil {
		log.Fatalf("budget-planning: %v", err)
	}
}
