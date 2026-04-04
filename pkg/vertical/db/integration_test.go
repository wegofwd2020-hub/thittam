//go:build integration
// +build integration

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// testDB opens a connection to the test database.
// Set THITTAM_TEST_DSN to override, e.g.:
//
//	THITTAM_TEST_DSN="postgres://localhost:5432/thittam_test?sslmode=disable"
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("THITTAM_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://localhost:5432/thittam_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	t.Cleanup(func() { db.Close() })
	return db
}

func TestIntegration_BindAndQueryVerticalConfig(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := NewStore(db)
	q := store.Queries()

	// 1. Upsert a vertical definition
	desc := "Integration test vertical"
	cfg := json.RawMessage(`{
		"entity_labels": {"project":"TestProject","project_plural":"TestProjects","phase":"Phase","phase_plural":"Phases","team_member":"Member","team_member_plural":"Members","rate_label":"Hourly","rate_unit":"hour"},
		"phase_types": [{"id":"init","label":"Init","order":1,"is_billable":false,"allowed_transitions":["done"]},{"id":"done","label":"Done","order":2,"is_billable":false,"allowed_transitions":[]}],
		"budget_categories": [],
		"budget_templates": [],
		"expense_categories": [],
		"approval_workflow": {"limits":[],"dual_approval_above":"0"},
		"inventory_categories": [],
		"report_definitions": [],
		"custom_fields": {"project":[],"expense":[]}
	}`)

	vd, err := q.UpsertVerticalDefinition(ctx, UpsertVerticalDefinitionParams{
		ID:          "integration-test",
		Name:        "Integration Test",
		Version:     "0.0.1",
		Description: &desc,
		Config:      cfg,
	})
	require.NoError(t, err)
	assert.Equal(t, "integration-test", vd.ID)
	assert.True(t, vd.IsActive)

	// 2. Bind a tenant to this vertical
	tenantID := uuid.New()
	adminID := uuid.New()
	err = q.BindTenantVertical(ctx, BindTenantVerticalParams{
		TenantID:       tenantID,
		VerticalID:     "integration-test",
		ConfigOverride: nil,
		RegisteredBy:   adminID,
	})
	require.NoError(t, err)

	// 3. Query via Store (the vertical.DB interface)
	data, err := store.GetVerticalConfigForTenant(ctx, tenantID)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// 4. Unmarshal into vertical.Config
	var vcfg vertical.Config
	require.NoError(t, json.Unmarshal(data, &vcfg))
	assert.Equal(t, "TestProject", vcfg.EntityLabels.Project)
	assert.Len(t, vcfg.PhaseTypes, 2)

	// 5. Test config override (deep merge)
	override := json.RawMessage(`{"entity_labels":{"project":"OverriddenProject"}}`)
	err = q.UpdateTenantVerticalOverride(ctx, UpdateTenantVerticalOverrideParams{
		TenantID:       tenantID,
		ConfigOverride: override,
	})
	require.NoError(t, err)

	data2, err := store.GetVerticalConfigForTenant(ctx, tenantID)
	require.NoError(t, err)
	var vcfg2 vertical.Config
	require.NoError(t, json.Unmarshal(data2, &vcfg2))
	assert.Equal(t, "OverriddenProject", vcfg2.EntityLabels.Project)

	// 6. Cleanup
	err = q.UnbindTenantVertical(ctx, tenantID)
	require.NoError(t, err)
	err = q.DeactivateVerticalDefinition(ctx, "integration-test")
	require.NoError(t, err)

	// 7. Verify ErrNotFound after unbind
	_, err = store.GetVerticalConfigForTenant(ctx, tenantID)
	require.Error(t, err)
	assert.ErrorIs(t, err, vertical.ErrNotFound)
}

func TestIntegration_ListVerticalDefinitions(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	q := New(db)

	rows, err := q.ListVerticalDefinitions(ctx)
	require.NoError(t, err)

	// If seed migration ran, we should have at least 4 verticals.
	if len(rows) >= 4 {
		ids := make(map[string]bool)
		for _, r := range rows {
			ids[r.ID] = true
		}
		assert.True(t, ids["movie-production"], "expected movie-production in list")
		assert.True(t, ids["software-development"], "expected software-development in list")
		assert.True(t, ids["construction"], "expected construction in list")
		assert.True(t, ids["events-management"], "expected events-management in list")
	}
}

func TestIntegration_SeedVerticalsUnmarshal(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	q := New(db)

	verticalIDs := []string{"movie-production", "software-development", "construction", "events-management"}
	for _, vid := range verticalIDs {
		t.Run(vid, func(t *testing.T) {
			vd, err := q.GetVerticalDefinition(ctx, vid)
			if err == sql.ErrNoRows {
				t.Skipf("vertical %s not seeded — run migration 003 first", vid)
			}
			require.NoError(t, err)

			// Unmarshal the config into a vertical.Config struct
			var cfg vertical.Config
			require.NoError(t, json.Unmarshal(vd.Config, &cfg), "config for %s should unmarshal cleanly", vid)
			assert.NotEmpty(t, cfg.EntityLabels.Project)
			assert.NotEmpty(t, cfg.PhaseTypes)
			assert.NotEmpty(t, cfg.BudgetCategories)
		})
	}
}
