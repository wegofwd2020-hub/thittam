// thittam-cli is the command-line tool for managing the Thittam platform.
// Primary use: validate and register vertical YAML definitions.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "thittam-cli",
		Short:   "Thittam platform management CLI",
		Version: version,
	}

	// vertical subcommand group
	verticalCmd := &cobra.Command{
		Use:   "vertical",
		Short: "Manage vertical definitions",
	}

	verticalCmd.AddCommand(
		newValidateCmd(),
		newRegisterCmd(),
		newListCmd(),
		newDeprecateCmd(),
	)

	root.AddCommand(verticalCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
