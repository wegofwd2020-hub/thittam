package demo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/wegofwd2020/thittam/pkg/registration"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// Generator builds a DemoPreview from a vertical config and template.
type Generator struct{}

// NewGenerator creates a demo data generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// GeneratePreview builds a complete preview of what will be created for a demo tenant.
func (g *Generator) GeneratePreview(cfg *vertical.Config, req PreviewRequest) (*DemoPreview, error) {
	tmpl, ok := templates[req.VerticalID]
	if !ok {
		return nil, fmt.Errorf("demo: no template for vertical %q", req.VerticalID)
	}

	companyName := req.CompanyName
	if companyName == "" {
		companyName = tmpl.CompanyName
	}

	slug := registration.Slugify(companyName)
	if slug == "" {
		slug = "demo-tenant"
	}

	// Generate a preview token
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("demo: generate token: %w", err)
	}

	// Build users with real email addresses using the slug
	users := make([]DemoUser, len(tmpl.Users))
	for i, u := range tmpl.Users {
		users[i] = DemoUser{
			Name:         u.Name,
			Email:        strings.ReplaceAll(u.Email, "{slug}", slug),
			Role:         u.Role,
			Department:   u.Department,
			TempPassword: "demo1234", // all demo accounts share this password
		}
	}

	plan := req.Plan
	if plan == "" {
		plan = "professional"
	}

	return &DemoPreview{
		PreviewToken: token,
		Tenant: DemoTenant{
			Name: companyName,
			Slug: slug,
			Plan: plan,
		},
		VerticalID:          cfg.ID,
		VerticalName:        cfg.Name,
		Users:               users,
		ChartOfAccounts:     cfg.DefaultChartOfAccounts,
		BudgetCategories:    cfg.BudgetCategories,
		BudgetTemplates:     cfg.BudgetTemplates,
		ExpenseCategories:   cfg.ExpenseCategories,
		ApprovalWorkflow:    cfg.ApprovalWorkflow,
		InventoryCategories: cfg.InventoryCategories,
		Projects:            tmpl.Projects,
		EntityLabels:        cfg.EntityLabels,
	}, nil
}

// VerticalIDs returns all vertical IDs that have demo templates.
func (g *Generator) VerticalIDs() []string {
	ids := make([]string, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	return ids
}

// HasTemplate returns true if a demo template exists for the given vertical.
func (g *Generator) HasTemplate(verticalID string) bool {
	_, ok := templates[verticalID]
	return ok
}

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
