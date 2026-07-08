package iam

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

func TestSetTenantRetention_IndefinitePause(t *testing.T) {
	t.Parallel()
	var gotHold *time.Time
	var gotReason string
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			gotHold, gotReason = holdUntil, freezeReason
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
		},
	}
	got, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "support escalation", false)
	require.NoError(t, err)
	assert.Nil(t, gotHold, "indefinite pause passes nil hold_until")
	assert.Equal(t, "support escalation", gotReason)
	require.NotNil(t, got.FreezeReason)
	assert.Equal(t, "support escalation", *got.FreezeReason)
}

func TestSetTenantRetention_ExtendUntilFutureDate(t *testing.T) {
	t.Parallel()
	future := time.Now().UTC().Add(60 * 24 * time.Hour)
	var gotHold *time.Time
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusGrace}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			gotHold = holdUntil
			return &Tenant{ID: id, Status: TenantStatusGrace, HoldUntil: holdUntil, FreezeReason: &freezeReason}, nil
		},
	}
	got, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, &future, "retention-extended: ticket-42", false)
	require.NoError(t, err)
	require.NotNil(t, gotHold)
	assert.Equal(t, future, *gotHold)
	assert.Equal(t, TenantStatusGrace, got.Status, "status must not regress")
}

func TestSetTenantRetention_RejectsNonHoldableStatus(t *testing.T) {
	t.Parallel()
	for _, st := range []string{TenantStatusActive, TenantStatusPurgeEligible} {
		st := st
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			repo := &mockRepo{
				getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
					return &Tenant{ID: id, Status: st}, nil
				},
				setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) {
					t.Fatal("SetTenantLegalHold must not be called for non-holdable status")
					return nil, nil
				},
			}
			_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "x", false)
			assert.ErrorIs(t, err, ErrTenantNotHoldable)
		})
	}
}

func TestSetTenantRetention_RejectsPastHoldUntil(t *testing.T) {
	t.Parallel()
	past := time.Now().UTC().Add(-time.Hour)
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) {
			t.Fatal("SetTenantLegalHold must not be called when hold_until is in the past")
			return nil, nil
		},
	}
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, &past, "x", false)
	assert.ErrorIs(t, err, ErrHoldUntilInPast)
}

func TestSetTenantRetention_CollisionRejectedWithoutOverwrite(t *testing.T) {
	t.Parallel()
	existing := "legal:case-42"
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &existing}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) {
			t.Fatal("must not overwrite an existing hold without overwrite=true")
			return nil, nil
		},
	}
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "retention-extended", false)
	assert.ErrorIs(t, err, ErrTenantHoldExists)
	assert.Contains(t, err.Error(), existing, "error must surface the existing freeze_reason")
}

func TestSetTenantRetention_CollisionAllowedWithOverwrite(t *testing.T) {
	t.Parallel()
	existing := "legal:case-42"
	called := false
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &existing}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			called = true
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &freezeReason}, nil
		},
	}
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "retention-extended", true)
	require.NoError(t, err)
	assert.True(t, called, "overwrite=true must proceed to the repo write")
}

func TestSetTenantRetention_RejectsNarrowingIndefiniteHoldToDated(t *testing.T) {
	t.Parallel()
	existing := "legal:case-42"
	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			// Indefinite hold: FreezeReason set, HoldUntil nil.
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &existing, HoldUntil: nil}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) {
			t.Fatal("must not write a dated hold over an existing indefinite hold")
			return nil, nil
		},
	}
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, &future, "retention-extended", true)
	assert.ErrorIs(t, err, ErrHoldNarrowsIndefinite)
}

func TestSetTenantRetention_IndefiniteToIndefinite_ReasonChangeAllowed(t *testing.T) {
	t.Parallel()
	existing := "legal:case-42"
	called := false
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			// Indefinite hold: FreezeReason set, HoldUntil nil.
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &existing, HoldUntil: nil}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			called = true
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
		},
	}
	// New holdUntil stays nil (indefinite -> indefinite); only the reason changes.
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "retention-extended: new reason", true)
	require.NoError(t, err)
	assert.True(t, called, "indefinite-to-indefinite (reason change only) must proceed to the repo write")
}

func TestSetTenantRetention_EmitsAuditWithOverwriteMeta(t *testing.T) {
	// Not t.Parallel() — waits on the audit flush.
	existing := "legal:case-42"
	before := &Tenant{ID: fixedTenantID, Status: TenantStatusSuspended, FreezeReason: &existing}
	newReason := "retention-extended: ticket-42"
	after := &Tenant{ID: fixedTenantID, Status: TenantStatusSuspended, FreezeReason: &newReason}
	repo := &mockRepo{
		getTenantFn:          func(_ context.Context, _ uuid.UUID) (*Tenant, error) { return before, nil },
		setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) { return after, nil },
	}
	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{BufferSize: 10, FlushInterval: 10 * time.Millisecond, BatchSize: 10}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	actorID := uuid.MustParse("a2000000-0000-0000-0000-000000000119")
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: actorID, Email: "admin@platform.internal", IP: "10.0.0.9"})

	_, err := svc.SetTenantRetention(ctx, fixedTenantID, nil, newReason, true)
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, audit.ActionLegalHoldApplied, e.Action)
	assert.Equal(t, audit.ResourceTenant, e.ResourceType)
	assert.Equal(t, fixedTenantID, e.TenantID)
	assert.Equal(t, actorID, e.ActorID)
	assert.JSONEq(t, `{"overwrote_previous":true,"previous_reason":"legal:case-42"}`, string(e.Metadata))
}
