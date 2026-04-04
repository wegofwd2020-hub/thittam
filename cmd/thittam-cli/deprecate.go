package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeprecateCmd() *cobra.Command {
	var (
		env   string
		token string
	)

	cmd := &cobra.Command{
		Use:   "deprecate <vertical-id>",
		Short: "Mark a vertical as inactive (blocks new registrations)",
		Long:  "Existing tenants are unaffected. New tenant registrations for this vertical are blocked.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeprecate(args[0], env, token)
		},
	}

	cmd.Flags().StringVar(&env, "env", "local", "Target environment: local, staging, production")
	cmd.Flags().StringVar(&token, "token", "", "Admin API token (or set THITTAM_ADMIN_TOKEN)")

	return cmd
}

func runDeprecate(id, env, token string) error {
	client, err := NewAdminClient(env, token)
	if err != nil {
		return err
	}

	fmt.Printf("Deprecating vertical %q in %s...\n", id, env)

	if err := client.DeprecateVertical(id); err != nil {
		return fmt.Errorf("deprecate failed: %w", err)
	}

	fmt.Printf("✓  Vertical %q is now inactive\n", id)
	fmt.Printf("   Existing tenants are unaffected. New registrations are blocked.\n\n")

	return nil
}
