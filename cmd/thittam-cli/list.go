package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var (
		env   string
		token string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered verticals",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(env, token)
		},
	}

	cmd.Flags().StringVar(&env, "env", "local", "Target environment: local, staging, production")
	cmd.Flags().StringVar(&token, "token", "", "Admin API token (or set THITTAM_ADMIN_TOKEN)")

	return cmd
}

func runList(env, token string) error {
	client, err := NewAdminClient(env, token)
	if err != nil {
		return err
	}

	items, err := client.ListVerticals()
	if err != nil {
		return fmt.Errorf("list verticals: %w", err)
	}

	if len(items) == 0 {
		fmt.Println("No verticals registered.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tVERSION\tSTATUS")
	for _, item := range items {
		status := "active"
		if !item.IsActive {
			status = "inactive"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Version, status)
	}
	w.Flush()

	return nil
}
