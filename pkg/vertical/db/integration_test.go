//go:build integration

// Integration tests for pkg/vertical/db.
//
// Uses pkg/testdb for connection + auto-rollback. Each test wraps its work in
// a transaction so writes never persist — re-runs are deterministic and the
// tests can run in parallel against the same database.

package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
)

func TestIntegration_BindAndQueryVerticalConfig(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	ctx := context.Background()

	store := verticaldb.NewStore(tx)
	q := store.Queries()

	// 1. Upsert a vertical definition (unique id so parallel runs don't collide)
	vid := "integration-test-" + uuid.New().String()[:8]
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

	vd, err := q.UpsertVerticalDefinition(ctx, verticaldb.UpsertVerticalDefinitionParams{
		ID:          vid,
		Name:        "Integration Test",
		Version:     "0.0.1",
		Description: pgtype.Text{String: "Integration test vertical", Valid: true},
		Config:      cfg,
	})
	require.NoError(t, err)
	assert.Equal(t, vid, vd.ID)
	assert.True(t, vd.IsActive)

	// 2. Bind a tenant to this vertical
	tenantID := uuid.New()
	adminID := uuid.New()
	err = q.BindTenantVertical(ctx, verticaldb.BindTenantVerticalParams{
		TenantID:       tenantID,
		VerticalID:     vid,
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
	err = q.UpdateTenantVerticalOverride(ctx, verticaldb.UpdateTenantVerticalOverrideParams{
		TenantID:       tenantID,
		ConfigOverride: override,
	})
	require.NoError(t, err)

	data2, err := store.GetVerticalConfigForTenant(ctx, tenantID)
	require.NoError(t, err)
	var vcfg2 vertical.Config
	require.NoError(t, json.Unmarshal(data2, &vcfg2))
	assert.Equal(t, "OverriddenProject", vcfg2.EntityLabels.Project)

	// 6. Verify ErrNotFound after unbind
	err = q.UnbindTenantVertical(ctx, tenantID)
	require.NoError(t, err)
	_, err = store.GetVerticalConfigForTenant(ctx, tenantID)
	require.Error(t, err)
	assert.ErrorIs(t, err, vertical.ErrNotFound)
}

func TestIntegration_ListVerticalDefinitions(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	ctx := context.Background()
	q := verticaldb.New(tx)

	rows, err := q.ListVerticalDefinitions(ctx)
	require.NoError(t, err)

	// If migration 003 (seed_vertical_definitions) ran, we should have all 4.
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
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	ctx := context.Background()
	q := verticaldb.New(tx)

	verticalIDs := []string{"movie-production", "software-development", "construction", "events-management"}
	for _, vid := range verticalIDs {
		vid := vid
		t.Run(vid, func(t *testing.T) {
			vd, err := q.GetVerticalDefinition(ctx, vid)
			if errors.Is(err, pgx.ErrNoRows) {
				t.Skipf("vertical %s not seeded — run migration 003 first", vid)
			}
			require.NoError(t, err)

			var cfg vertical.Config
			require.NoError(t, json.Unmarshal(vd.Config, &cfg), "config for %s should unmarshal cleanly", vid)
			assert.NotEmpty(t, cfg.EntityLabels.Project)
			assert.NotEmpty(t, cfg.PhaseTypes)
			assert.NotEmpty(t, cfg.BudgetCategories)
		})
	}
}
