package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/wegofwd2020/thittam/services/billing"
)

const deadListLimit = 100

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List dead-lettered outbox events, most recent first",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext()
			defer cancel()

			repo, closeFn, err := openRepo(ctx)
			if err != nil {
				return err
			}
			defer closeFn()

			dead, err := repo.ListDeadOutbox(ctx, deadListLimit)
			if err != nil {
				return err
			}
			if len(dead) == 0 {
				fmt.Println("dead-letter queue is empty")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSUBJECT\tTENANT\tATTEMPTS\tLAST ERROR")
			for _, e := range dead {
				lastErr := ""
				if e.LastError != nil {
					lastErr = truncate(*e.LastError, 60)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", e.ID, e.Subject, e.TenantID, e.Attempts, lastErr)
			}
			return w.Flush()
		},
	}
}

func newReplayCmd() *cobra.Command {
	var idFlag string
	var all bool

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Re-arm dead-lettered events so the relay retries them",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Exactly one of --id / --all. Both set or neither set is an error.
			if (idFlag != "") == all {
				return errors.New("specify exactly one of --id or --all")
			}

			ctx, cancel := cmdContext()
			defer cancel()

			repo, closeFn, err := openRepo(ctx)
			if err != nil {
				return err
			}
			defer closeFn()

			if idFlag != "" {
				id, perr := uuid.Parse(idFlag)
				if perr != nil {
					return fmt.Errorf("invalid --id: %w", perr)
				}
				if rerr := repo.ReplayDeadOutbox(ctx, id); rerr != nil {
					if errors.Is(rerr, billing.ErrOutboxEventNotFound) {
						return fmt.Errorf("no dead event with id %s", id)
					}
					return rerr
				}
				fmt.Printf("replayed %s\n", id)
				return nil
			}

			dead, lerr := repo.ListDeadOutbox(ctx, deadListLimit)
			if lerr != nil {
				return lerr
			}
			if len(dead) == 0 {
				fmt.Println("dead-letter queue is empty; nothing to replay")
				return nil
			}
			for _, e := range dead {
				if rerr := repo.ReplayDeadOutbox(ctx, e.ID); rerr != nil {
					return fmt.Errorf("replay %s: %w", e.ID, rerr)
				}
				fmt.Printf("replayed %s\n", e.ID)
			}
			fmt.Printf("replayed %d event(s)\n", len(dead))
			return nil
		},
	}

	cmd.Flags().StringVar(&idFlag, "id", "", "UUID of a single dead event to replay")
	cmd.Flags().BoolVar(&all, "all", false, "Replay every dead event")
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
