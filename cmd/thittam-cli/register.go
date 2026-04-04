package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

func newRegisterCmd() *cobra.Command {
	var (
		env    string
		token  string
		dryRun bool
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "register <file.yaml>",
		Short: "Validate and register a vertical definition",
		Long:  "Validates the YAML file, then calls the admin API to register the vertical.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegister(args[0], env, token, dryRun, force)
		},
	}

	cmd.Flags().StringVar(&env, "env", "local", "Target environment: local, staging, production")
	cmd.Flags().StringVar(&token, "token", "", "Admin API token (or set THITTAM_ADMIN_TOKEN)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and print JSON payload without calling the API")
	cmd.Flags().BoolVar(&force, "force", false, "Allow re-registering an existing vertical ID")

	return cmd
}

func runRegister(filename, env, token string, dryRun, force bool) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Step 1: Validate
	fmt.Print("Validating... ")
	errs, err := vertical.Validate(data)
	if err != nil {
		fmt.Println("✗")
		return fmt.Errorf("parse error: %w", err)
	}
	if len(errs) > 0 {
		fmt.Printf("✗ (%d errors)\n\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  ✗  [%s] %s\n", e.Field, e.Message)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
	fmt.Println("✓")

	// Step 2: Parse the YAML to build the API payload
	var def vertical.VerticalDefinition
	if err := yamlUnmarshal(data, &def); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}

	v := def.Vertical

	// Build the config JSON (everything except id/name/version/description)
	configJSON, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	payload := RegisterVerticalRequest{
		ID:          v.ID,
		Name:        v.Name,
		Version:     v.Version,
		Description: v.Description,
		Config:      configJSON,
	}

	// Step 3: Dry-run or register
	if dryRun {
		prettyJSON, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Printf("\n--dry-run: JSON payload that would be sent:\n\n%s\n\n", string(prettyJSON))
		return nil
	}

	client, err := NewAdminClient(env, token)
	if err != nil {
		return err
	}

	fmt.Printf("Registering %s v%s in %s...\n", v.ID, v.Version, env)

	if err := client.RegisterVertical(payload, force); err != nil {
		return fmt.Errorf("register failed: %w", err)
	}

	fmt.Printf("✓  Vertical registered successfully\n")
	fmt.Printf("✓  Available immediately for new tenant registrations\n")
	fmt.Printf("   ID: %s | Name: %s\n\n", v.ID, v.Name)

	return nil
}
