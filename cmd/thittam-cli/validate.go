package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file.yaml>",
		Short: "Validate a vertical YAML definition",
		Long:  "Validates a vertical YAML file against all schema and business rules before registration.",
		Args:  cobra.ExactArgs(1),
		RunE:  runValidate,
	}
}

func runValidate(cmd *cobra.Command, args []string) error {
	filename := args[0]

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	errs, err := vertical.Validate(data)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "\nValidation failed — %d error(s):\n\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  ✗  [%s] %s\n", e.Field, e.Message)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	// Parse again to extract summary info for the success output
	summary, err := parseVerticalSummary(data)
	if err != nil {
		return fmt.Errorf("parse summary: %w", err)
	}

	printValidationSuccess(summary)
	return nil
}

type verticalSummary struct {
	ID                  string
	Name                string
	Version             string
	PhaseCount          int
	BudgetCatCount      int
	BudgetTemplateCount int
	ExpenseCatCount     int
	InventoryCatCount   int
	CoACount            int
	ReportCount         int
}

func parseVerticalSummary(data []byte) (*verticalSummary, error) {
	// Use the validator's YAML struct to get all fields including CoA
	var def vertical.VerticalDefinition
	if err := yamlUnmarshal(data, &def); err != nil {
		return nil, err
	}

	v := def.Vertical
	return &verticalSummary{
		ID:                  v.ID,
		Name:                v.Name,
		Version:             v.Version,
		PhaseCount:          len(v.PhaseTypes),
		BudgetCatCount:      len(v.BudgetCategories),
		BudgetTemplateCount: len(v.BudgetTemplates),
		ExpenseCatCount:     len(v.ExpenseCategories),
		InventoryCatCount:   len(v.InventoryCategories),
		CoACount:            len(v.DefaultChartOfAccounts),
		ReportCount:         len(v.ReportDefinitions),
	}, nil
}

func printValidationSuccess(s *verticalSummary) {
	fmt.Printf("\n")
	fmt.Printf("  ✓  id:                   %s\n", s.ID)
	fmt.Printf("  ✓  name:                 %s\n", s.Name)
	fmt.Printf("  ✓  version:              %s\n", s.Version)
	fmt.Printf("  ✓  phase_types:          %d phases, acyclic transition graph ✓\n", s.PhaseCount)
	fmt.Printf("  ✓  budget_categories:    %d categories\n", s.BudgetCatCount)
	fmt.Printf("  ✓  budget_templates:     %d templates\n", s.BudgetTemplateCount)
	fmt.Printf("  ✓  expense_categories:   %d categories\n", s.ExpenseCatCount)
	fmt.Printf("  ✓  inventory_categories: %d categories\n", s.InventoryCatCount)
	if s.CoACount > 0 {
		fmt.Printf("  ✓  chart_of_accounts:    %d accounts, all category codes referenced ✓\n", s.CoACount)
	}
	fmt.Printf("  ✓  report_definitions:   %d reports\n", s.ReportCount)
	fmt.Printf("\n  Validation passed — ready to register.\n\n")
}
