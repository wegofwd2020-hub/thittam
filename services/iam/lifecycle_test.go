package iam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deterministic fixtures (Rule #9: deterministic IDs, fixed timestamps).
var (
	lifecycleTenantID = uuid.MustParse("e1000000-0000-0000-0000-000000000092")
	lifecycleNow      = time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
)

// t0 returns a *time.Time shifted by the given duration before lifecycleNow.
func t0(d time.Duration) *time.Time {
	v := lifecycleNow.Add(-d)
	return &v
}

func TestNextLifecycleStatus_TableDriven(t *testing.T) {
	t.Parallel()

	day := 24 * time.Hour
	cases := []struct {
		name       string
		tenant     Tenant
		want       string
		wantMoved  bool
	}{
		{
			name:      "active tenant is never moved",
			tenant:    Tenant{Status: TenantStatusActive},
			wantMoved: false,
		},
		{
			name:      "suspended but clock not started (nil suspended_at) — no move",
			tenant:    Tenant{Status: TenantStatusSuspended, SuspendedAt: nil},
			wantMoved: false,
		},
		{
			name:      "suspended 29 days — below SuspensionGracePeriod, no move",
			tenant:    Tenant{Status: TenantStatusSuspended, SuspendedAt: t0(29 * day)},
			wantMoved: false,
		},
		{
			name:      "suspended 30 days — exactly at threshold, moves to grace",
			tenant:    Tenant{Status: TenantStatusSuspended, SuspendedAt: t0(30 * day)},
			want:      TenantStatusGrace,
			wantMoved: true,
		},
		{
			name:      "suspended 45 days — past threshold, moves to grace",
			tenant:    Tenant{Status: TenantStatusSuspended, SuspendedAt: t0(45 * day)},
			want:      TenantStatusGrace,
			wantMoved: true,
		},
		{
			name:      "grace 45 days since suspension — below GraceToDeactivated, no move",
			tenant:    Tenant{Status: TenantStatusGrace, SuspendedAt: t0(45 * day)},
			wantMoved: false,
		},
		{
			name:      "grace 90 days since suspension — exactly at threshold, moves to deactivated",
			tenant:    Tenant{Status: TenantStatusGrace, SuspendedAt: t0(90 * day)},
			want:      TenantStatusDeactivated,
			wantMoved: true,
		},
		{
			name:      "deactivated 100 days — below DeactivatedToPurge, no move",
			tenant:    Tenant{Status: TenantStatusDeactivated, DeactivatedAt: t0(100 * day)},
			wantMoved: false,
		},
		{
			name:      "deactivated 180 days — exactly at threshold, moves to purge_eligible",
			tenant:    Tenant{Status: TenantStatusDeactivated, DeactivatedAt: t0(180 * day)},
			want:      TenantStatusPurgeEligible,
			wantMoved: true,
		},
		{
			name:      "deactivated without deactivated_at — no move (defensive)",
			tenant:    Tenant{Status: TenantStatusDeactivated, DeactivatedAt: nil},
			wantMoved: false,
		},
		{
			name:      "purge_eligible is terminal — no further move",
			tenant:    Tenant{Status: TenantStatusPurgeEligible, DeactivatedAt: t0(365 * day)},
			wantMoved: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, moved := nextLifecycleStatus(&tc.tenant, lifecycleNow)
			assert.Equal(t, tc.wantMoved, moved, "wantMoved")
			if tc.wantMoved {
				assert.Equal(t, tc.want, got, "next status")
			}
		})
	}
}

func TestAdvanceTenantLifecycle_SuspendedToGrace(t *testing.T) {
	t.Parallel()

	day := 24 * time.Hour
	suspended := t0(31 * day)

	repo := &mockRepo{
		getTenantFn: func(ctx context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{
				ID:          id,
				Name:        "Acme Studios",
				Status:      TenantStatusSuspended,
				SuspendedAt: suspended,
			}, nil
		},
		transitionTenantStatusFn: func(ctx context.Context, id uuid.UUID, from, to string) (*Tenant, bool, error) {
			assert.Equal(t, TenantStatusSuspended, from)
			assert.Equal(t, TenantStatusGrace, to)
			return &Tenant{ID: id, Name: "Acme Studios", Status: to, SuspendedAt: suspended}, true, nil
		},
	}

	s := &Service{repo: repo}
	trans, err := s.AdvanceTenantLifecycle(context.Background(), lifecycleTenantID, lifecycleNow)
	require.NoError(t, err)
	require.NotNil(t, trans)
	assert.Equal(t, TenantStatusSuspended, trans.FromStatus)
	assert.Equal(t, TenantStatusGrace, trans.ToStatus)
	assert.Equal(t, "Acme Studios", trans.TenantName)
	assert.Equal(t, lifecycleNow, trans.OccurredAt)
}

func TestAdvanceTenantLifecycle_NotYetDue_ReturnsNilTransition(t *testing.T) {
	t.Parallel()

	day := 24 * time.Hour
	suspended := t0(10 * day)

	transitionCalled := false
	repo := &mockRepo{
		getTenantFn: func(ctx context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended, SuspendedAt: suspended}, nil
		},
		transitionTenantStatusFn: func(ctx context.Context, id uuid.UUID, from, to string) (*Tenant, bool, error) {
			transitionCalled = true
			return nil, false, nil
		},
	}

	s := &Service{repo: repo}
	trans, err := s.AdvanceTenantLifecycle(context.Background(), lifecycleTenantID, lifecycleNow)
	require.NoError(t, err)
	assert.Nil(t, trans, "no transition when below threshold")
	assert.False(t, transitionCalled, "repo.TransitionTenantStatus must not be invoked when no-op")
}

func TestAdvanceTenantLifecycle_RaceLoser_ReturnsNilTransition(t *testing.T) {
	t.Parallel()

	// The tenant is due for transition at Get() time, but between our read
	// and the conditional UPDATE, another worker wins the race and advances
	// it. The status-guarded UPDATE matches zero rows → (nil, false, nil).
	day := 24 * time.Hour
	suspended := t0(45 * day)

	repo := &mockRepo{
		getTenantFn: func(ctx context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended, SuspendedAt: suspended}, nil
		},
		transitionTenantStatusFn: func(ctx context.Context, id uuid.UUID, from, to string) (*Tenant, bool, error) {
			return nil, false, nil
		},
	}

	s := &Service{repo: repo}
	trans, err := s.AdvanceTenantLifecycle(context.Background(), lifecycleTenantID, lifecycleNow)
	require.NoError(t, err)
	assert.Nil(t, trans, "losing the status race is a no-op, not an error")
}

func TestAdvanceTenantLifecycle_Idempotent_ActiveTenant(t *testing.T) {
	t.Parallel()

	transitionCalled := false
	repo := &mockRepo{
		getTenantFn: func(ctx context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusActive}, nil
		},
		transitionTenantStatusFn: func(ctx context.Context, id uuid.UUID, from, to string) (*Tenant, bool, error) {
			transitionCalled = true
			return nil, false, nil
		},
	}

	s := &Service{repo: repo}
	for i := 0; i < 3; i++ {
		trans, err := s.AdvanceTenantLifecycle(context.Background(), lifecycleTenantID, lifecycleNow)
		require.NoError(t, err)
		assert.Nil(t, trans, "active tenant never transitions, iteration %d", i)
	}
	assert.False(t, transitionCalled, "no repo UPDATE should be attempted for active tenants")
}

func TestAdvanceTenantLifecycle_GetTenantError_Propagated(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("db unavailable")
	repo := &mockRepo{
		getTenantFn: func(ctx context.Context, id uuid.UUID) (*Tenant, error) {
			return nil, sentinel
		},
	}

	s := &Service{repo: repo}
	trans, err := s.AdvanceTenantLifecycle(context.Background(), lifecycleTenantID, lifecycleNow)
	assert.Nil(t, trans)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}
