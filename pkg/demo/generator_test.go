package demo

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

func movieConfig() *vertical.Config {
	return &vertical.Config{
		ID: "movie-production", Name: "Movie & Film Production", Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project: "Production", ProjectPlural: "Productions",
			Phase: "Phase", PhasePlural: "Phases",
			TeamMember: "Crew Member", TeamMemberPlural: "Crew Members",
			RateLabel: "Day Rate", RateUnit: "day",
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", DefaultAccountCode: "5100"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(200000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true},
		},
		DefaultChartOfAccounts: []vertical.ChartOfAccountEntry{
			{Code: "1000", Name: "Cash and Bank", AccountType: "asset"},
			{Code: "5100", Name: "Above the Line Expenses", AccountType: "expense"},
		},
	}
}

func TestGeneratePreview_MovieProduction(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	preview, err := gen.GeneratePreview(movieConfig(), PreviewRequest{
		VerticalID:  "movie-production",
		CompanyName: "Test Studios",
		Plan:        "professional",
	})
	require.NoError(t, err)

	assert.Equal(t, "movie-production", preview.VerticalID)
	assert.Equal(t, "Movie & Film Production", preview.VerticalName)
	assert.Equal(t, "Test Studios", preview.Tenant.Name)
	assert.Equal(t, "test-studios", preview.Tenant.Slug)
	assert.Equal(t, "professional", preview.Tenant.Plan)
	assert.NotEmpty(t, preview.PreviewToken)

	// Users from movie template
	assert.Len(t, preview.Users, 8)
	assert.Equal(t, "Rajesh Kumar", preview.Users[0].Name)
	assert.Equal(t, "super_admin", preview.Users[0].Role)
	assert.Contains(t, preview.Users[0].Email, "test-studios")
	assert.Equal(t, "demo1234", preview.Users[0].TempPassword)

	// Vertical config passed through
	assert.Len(t, preview.ChartOfAccounts, 2)
	assert.Len(t, preview.BudgetCategories, 1)
	assert.Equal(t, "Production", preview.EntityLabels.Project)

	// Projects from template
	assert.Len(t, preview.Projects, 3)
	assert.Equal(t, "The Last Horizon", preview.Projects[0].Title)
	assert.Equal(t, "production", preview.Projects[0].Phase)
}

func TestGeneratePreview_AllVerticals(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	verticals := []string{"movie-production", "software-development", "construction", "events-management"}
	for _, vid := range verticals {
		vid := vid
		t.Run(vid, func(t *testing.T) {
			t.Parallel()
			cfg := &vertical.Config{ID: vid, Name: vid}
			preview, err := gen.GeneratePreview(cfg, PreviewRequest{
				VerticalID: vid,
				Plan:       "starter",
			})
			require.NoError(t, err)
			assert.NotEmpty(t, preview.Users, "vertical %s should have demo users", vid)
			assert.NotEmpty(t, preview.Projects, "vertical %s should have demo projects", vid)
			assert.NotEmpty(t, preview.PreviewToken)
		})
	}
}

func TestGeneratePreview_DefaultCompanyName(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	preview, err := gen.GeneratePreview(movieConfig(), PreviewRequest{
		VerticalID: "movie-production",
		// CompanyName omitted — should use template default
	})
	require.NoError(t, err)
	assert.Equal(t, "Sunrise Studios", preview.Tenant.Name)
	assert.Equal(t, "sunrise-studios", preview.Tenant.Slug)
}

func TestGeneratePreview_DefaultPlan(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	preview, err := gen.GeneratePreview(movieConfig(), PreviewRequest{
		VerticalID: "movie-production",
	})
	require.NoError(t, err)
	assert.Equal(t, "professional", preview.Tenant.Plan) // default
}

func TestGeneratePreview_UnknownVertical(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	_, err := gen.GeneratePreview(&vertical.Config{ID: "unknown"}, PreviewRequest{
		VerticalID: "unknown",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no template")
}

func TestHasTemplate(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	assert.True(t, gen.HasTemplate("movie-production"))
	assert.True(t, gen.HasTemplate("software-development"))
	assert.True(t, gen.HasTemplate("construction"))
	assert.True(t, gen.HasTemplate("events-management"))
	assert.False(t, gen.HasTemplate("unknown"))
}

func TestVerticalIDs(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()
	ids := gen.VerticalIDs()
	assert.Len(t, ids, 4)
}

func TestGeneratePreview_SoftwareDev(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	cfg := &vertical.Config{
		ID: "software-development", Name: "Software Development Services",
		EntityLabels: vertical.EntityLabels{Project: "Project", TeamMember: "Team Member"},
	}

	preview, err := gen.GeneratePreview(cfg, PreviewRequest{
		VerticalID:  "software-development",
		CompanyName: "Acme Tech",
	})
	require.NoError(t, err)
	assert.Len(t, preview.Users, 7) // software template has 7 users
	assert.Len(t, preview.Projects, 4) // 4 projects
	assert.Equal(t, "E-Commerce Platform v2", preview.Projects[0].Title)
}

func TestGeneratePreview_Construction(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	preview, err := gen.GeneratePreview(&vertical.Config{ID: "construction"}, PreviewRequest{
		VerticalID: "construction",
	})
	require.NoError(t, err)
	assert.Len(t, preview.Users, 6)
	assert.Len(t, preview.Projects, 3)
	assert.Equal(t, "Prestige Towers — Phase 2", preview.Projects[0].Title)
}

func TestGeneratePreview_Events(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	preview, err := gen.GeneratePreview(&vertical.Config{ID: "events-management"}, PreviewRequest{
		VerticalID: "events-management",
	})
	require.NoError(t, err)
	assert.Len(t, preview.Users, 6)
	assert.Len(t, preview.Projects, 3)
	assert.Equal(t, "TechSummit 2026 — Annual Conference", preview.Projects[0].Title)
}

func TestGeneratePreview_EmailsUseSlug(t *testing.T) {
	t.Parallel()
	gen := NewGenerator()

	preview, err := gen.GeneratePreview(movieConfig(), PreviewRequest{
		VerticalID:  "movie-production",
		CompanyName: "My Company",
	})
	require.NoError(t, err)

	for _, user := range preview.Users {
		assert.Contains(t, user.Email, "my-company", "email should contain slug")
		assert.Contains(t, user.Email, ".demo", "email should end with .demo")
	}
}
