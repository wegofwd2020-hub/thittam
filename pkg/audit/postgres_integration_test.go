//go:build integration

package audit_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestPostgresAudit_RoundTrip(t *testing.T) {
	pool := testdb.Open(t)
	store := audit.NewPostgres(pool)
	tenant := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
	})

	e := audit.Event{
		TenantID:     tenant,
		ActorID:      uuid.New(),
		ActorEmail:   "system:test",
		Action:       audit.ActionStatusChanged,
		ResourceType: audit.ResourceTenant,
		ResourceID:   tenant,
		OldState:     json.RawMessage(`{"status":"suspended"}`),
		NewState:     json.RawMessage(`{"status":"grace"}`),
		OccurredAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, store.Insert(context.Background(), e))

	got, err := store.Query(context.Background(), audit.QueryFilter{TenantID: tenant})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, e.ActorEmail, got[0].ActorEmail)
	assert.Equal(t, audit.ActionStatusChanged, got[0].Action)
	assert.JSONEq(t, `{"status":"grace"}`, string(got[0].NewState))
	assert.Empty(t, got[0].ActorIP)         // NULL → zero
	assert.Nil(t, got[0].ProductionID)      // NULL → nil
	assert.NotEqual(t, uuid.Nil, got[0].ID) // DB default assigned
}

func TestPostgresAudit_BatchAndFilter(t *testing.T) {
	pool := testdb.Open(t)
	store := audit.NewPostgres(pool)
	tenant := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
	})

	base := time.Now().UTC()
	mk := func(a audit.Action, at time.Time) audit.Event {
		return audit.Event{TenantID: tenant, ActorID: uuid.New(), ActorEmail: "system:test",
			Action: a, ResourceType: audit.ResourceTenant, ResourceID: tenant, OccurredAt: at}
	}
	require.NoError(t, store.InsertBatch(context.Background(), []audit.Event{
		mk(audit.ActionStatusChanged, base.Add(-2*time.Hour)),
		mk(audit.ActionStatusChanged, base.Add(-1*time.Hour)),
		mk(audit.ActionLegalHoldApplied, base),
	}))

	// filter by action
	sc := audit.ActionStatusChanged
	got, err := store.Query(context.Background(), audit.QueryFilter{TenantID: tenant, Action: &sc})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	// DESC order
	assert.True(t, got[0].OccurredAt.After(got[1].OccurredAt))

	// limit/offset
	page, err := store.Query(context.Background(), audit.QueryFilter{TenantID: tenant, Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, page, 1)
}
