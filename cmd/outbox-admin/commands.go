package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"unicode/utf8"

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
			if err := w.Flush(); err != nil {
				return err
			}

			// The dead count is decoration on top of the listing above,
			// which already succeeded: a transient failure here must not
			// turn a successful `list` into a runtime error (finding 2).
			var total int64
			stats, serr := repo.OutboxStats(ctx)
			if serr == nil {
				total = stats.Dead
			}
			remaining, ok, warn := deadCountNotice(len(dead), total, serr)
			if warn != "" {
				fmt.Fprintln(os.Stderr, warn)
			}
			if ok {
				fmt.Printf("\nNOTE: showing %d of %d dead-lettered event(s); %d NOT shown. Re-run list after handling these to see the rest.\n",
					len(dead), stats.Dead, remaining)
			}
			return nil
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

			var id uuid.UUID
			if idFlag != "" {
				parsed, perr := uuid.Parse(idFlag)
				if perr != nil {
					return fmt.Errorf("invalid --id: %w", perr)
				}
				id = parsed
			}

			ctx, cancel := cmdContext()
			defer cancel()

			repo, closeFn, err := openRepo(ctx)
			if err != nil {
				return err
			}
			defer closeFn()

			if idFlag != "" {
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

			// Snapshot the true total before mutating anything, so we can
			// tell the operator whether this batch is capped short of the
			// full DLQ (finding 1), independent of any per-event failures
			// below (finding 3). This count is purely informational: a
			// transient failure here must not stop the replay loop below
			// from running (finding 2) — ListDeadOutbox already succeeded,
			// so the actual replay work must proceed regardless.
			var total int64
			stats, serr := repo.OutboxStats(ctx)
			if serr == nil {
				total = stats.Dead
			}
			cappedRemaining, cappedOK, warn := deadCountNotice(len(dead), total, serr)
			if warn != "" {
				fmt.Fprintln(os.Stderr, warn)
			}

			var replayed, failed int
			for _, e := range dead {
				if rerr := repo.ReplayDeadOutbox(ctx, e.ID); rerr != nil {
					failed++
					fmt.Fprintf(os.Stderr, "FAILED to replay %s: %v\n", e.ID, rerr)
					continue
				}
				replayed++
				fmt.Printf("replayed %s\n", e.ID)
			}
			fmt.Printf("summary: replayed %d event(s), failed %d event(s)\n", replayed, failed)

			if cappedOK {
				fmt.Printf("NOTE: %d dead-lettered event(s) were NOT attempted this run (capped at %d). Re-run replay --all to continue draining.\n",
					cappedRemaining, len(dead))
			}

			if failed > 0 {
				return fmt.Errorf("%d of %d event(s) failed to replay", failed, len(dead))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idFlag, "id", "", "UUID of a single dead event to replay")
	cmd.Flags().BoolVar(&all, "all", false, "Replay every dead event")
	return cmd
}

// deadCountNotice decides whether an operator-facing "N more remaining"
// notice is warranted, given a best-effort OutboxStats snapshot. It is pure
// so the decision (used identically by `list` and `replay --all`) can be
// unit-tested without a database.
//
//   - shown is the number of dead events the caller already listed/attempted
//     (len(dead)); total is stats.Dead from a successful OutboxStats call
//     (ignored/zero if statsErr is non-nil).
//   - If statsErr is non-nil, the true total is unknowable this run: ok is
//     false and warn explains why, so an operator who sees no notice does
//     not conclude the queue is drained — they know the count could not be
//     determined, rather than reading silence as "drained" or "zero".
//   - If statsErr is nil and total <= shown, there is nothing beyond what
//     was already shown/attempted: ok is false, warn is "".
//   - Otherwise ok is true and remaining is total-shown, for the caller to
//     fold into its own (list- or replay-specific) wording.
func deadCountNotice(shown int, total int64, statsErr error) (remaining int64, ok bool, warn string) {
	if statsErr != nil {
		return 0, false, fmt.Sprintf(
			"WARNING: could not determine total dead-lettered event count (%v); a \"N more remaining\" notice may be missing even though more events exist below the display/replay cap. Do not read the absence of that notice as the queue being drained.",
			statsErr)
	}
	if total <= int64(shown) {
		return 0, false, ""
	}
	return total - int64(shown), true, ""
}

// truncate returns s unchanged if it has at most n runes; otherwise it cuts s
// to n runes total (n-1 runes of content plus a trailing ellipsis), operating
// on runes so a multi-byte UTF-8 sequence is never split mid-rune. n <= 0
// yields an empty string rather than panicking.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-1]) + "…"
}
