// outbox-admin inspects and drains the billing outbox dead-letter queue (#134).
//
// A dead-lettered event is one the relay failed to publish maxOutboxAttempts
// times while other events in its batch succeeded — i.e. the event itself is
// bad, not the broker. Nothing drains event_outbox_dead automatically: a human
// must understand why the event failed before re-arming it. That is the point.
//
// Replaying a suspension event matters. Until it is delivered, iam's consumer
// never starts that tenant's retention clock.
//
// Run with the RUNTIME database credential (thittam_app). This binary performs
// only DML; it does not need — and must not be given — the owner DSN that
// cmd/purge-worker requires.
//
// Exit codes: 0 success; 1 config error (missing DATABASE_URL); 2 runtime error.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	billingdb "github.com/wegofwd2020/thittam/services/billing/db"
)

func main() {
	root := &cobra.Command{
		Use:   "outbox-admin",
		Short: "Inspect and replay billing outbox dead-letter events",
	}
	root.AddCommand(newListCmd(), newReplayCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

// openRepo connects using DATABASE_URL. The caller owns the returned close func.
func openRepo(ctx context.Context) (*billingdb.Postgres, func(), error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "ERROR: DATABASE_URL is required")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}
	return billingdb.NewPostgres(pool), pool.Close, nil
}

func cmdContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}
