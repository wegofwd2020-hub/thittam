// expense-tracking service entrypoint.
// Registers the vertical interceptor and expense service handlers.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/expense"
)

func main() {
	// In production, the Loader would be wired with Redis + DB.
	// For now, we pass nil (vertical interceptor skipped) to allow startup.
	srv := server.New(server.Config{
		Name:        "expense-tracking",
		Port:        8082,
		MetricsPort: 9092,
		// Loader: vertical.NewLoader(redisClient, dbStore, nil),
	}, nil)

	// Create service with nil repo (to be wired with real DB)
	_ = expense.NewHandler(expense.NewService(nil))

	// Register gRPC service here when proto generation is set up:
	// expensepb.RegisterExpenseServiceServer(srv.GRPCServer(), handler)

	log.Printf("expense-tracking service ready")
	if err := srv.Run(); err != nil {
		log.Fatalf("expense-tracking: %v", err)
	}
}
