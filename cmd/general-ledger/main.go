// general-ledger service entrypoint.
// Implements double-entry accounting: chart of accounts, journal entries,
// trial balance, and period closing. Source of truth for all financials.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/ledger"
)

func main() {
	srv := server.New(server.Config{
		Name:        "general-ledger",
		Port:        8083,
		MetricsPort: 9093,
	}, nil)

	// In production, wire the postgres-backed repository:
	//   repo — pgx-backed ledger.Repository
	//
	// The LedgerSeeder adapter (for pkg/registration) is a gRPC client
	// that calls this service's SeedChartOfAccounts and OpenAccountingPeriod RPCs.
	_ = ledger.NewHandler(ledger.NewService(nil))

	// Register gRPC service here when proto generation is set up:
	// ledgerpb.RegisterLedgerServiceServer(srv.GRPCServer(), handler)

	log.Printf("general-ledger service ready on :8083")
	if err := srv.Run(); err != nil {
		log.Fatalf("general-ledger: %v", err)
	}
}
