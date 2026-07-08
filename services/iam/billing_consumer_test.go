package iam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/events"
)

// fakeSuspender is a test double for tenantSuspender. No DB involved.
type fakeSuspender struct {
	getTenantFn func(ctx context.Context, id uuid.UUID) (*Tenant, error)
	suspendFn   func(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason *string) (*Tenant, error)

	suspendCalls []uuid.UUID
}

func (f *fakeSuspender) GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	return f.getTenantFn(ctx, id)
}

func (f *fakeSuspender) SuspendTenant(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason *string) (*Tenant, error) {
	f.suspendCalls = append(f.suspendCalls, id)
	return f.suspendFn(ctx, id, holdUntil, freezeReason)
}

func tenantWithStatus(status string) func(context.Context, uuid.UUID) (*Tenant, error) {
	return func(_ context.Context, id uuid.UUID) (*Tenant, error) {
		return &Tenant{ID: id, Status: status}, nil
	}
}

func TestBillingConsumer_ActiveTenant_Suspends(t *testing.T) {
	fake := &fakeSuspender{
		getTenantFn: tenantWithStatus(TenantStatusActive),
		suspendFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason *string) (*Tenant, error) {
			require.Nil(t, holdUntil, "handler must pass a nil hold — a freeze_reason would freeze the retention sweeper")
			require.Nil(t, freezeReason)
			return &Tenant{ID: id, Status: TenantStatusSuspended}, nil
		},
	}
	h := NewBillingConsumer(fake)
	tenantID := uuid.New()
	env := &events.Envelope{Type: events.SubjectBillingSubscriptionSuspended, TenantID: tenantID}

	err := h.Handle(context.Background(), env)

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{tenantID}, fake.suspendCalls)
}

func TestBillingConsumer_AlreadySuspended_NoOp(t *testing.T) {
	fake := &fakeSuspender{
		getTenantFn: tenantWithStatus(TenantStatusSuspended),
		suspendFn: func(_ context.Context, id uuid.UUID, _ *time.Time, _ *string) (*Tenant, error) {
			t.Fatal("SuspendTenant must not be called for an already-suspended tenant")
			return nil, nil
		},
	}
	h := NewBillingConsumer(fake)
	env := &events.Envelope{Type: events.SubjectBillingSubscriptionSuspended, TenantID: uuid.New()}

	err := h.Handle(context.Background(), env)

	require.NoError(t, err)
	require.Empty(t, fake.suspendCalls)
}

func TestBillingConsumer_NonActiveStatuses_NoRegression(t *testing.T) {
	for _, status := range []string{TenantStatusGrace, TenantStatusDeactivated, TenantStatusPurgeEligible} {
		t.Run(status, func(t *testing.T) {
			fake := &fakeSuspender{
				getTenantFn: tenantWithStatus(status),
				suspendFn: func(_ context.Context, id uuid.UUID, _ *time.Time, _ *string) (*Tenant, error) {
					t.Fatalf("SuspendTenant must not be called for tenant status %q", status)
					return nil, nil
				},
			}
			h := NewBillingConsumer(fake)
			env := &events.Envelope{Type: events.SubjectBillingSubscriptionSuspended, TenantID: uuid.New()}

			err := h.Handle(context.Background(), env)

			require.NoError(t, err)
			require.Empty(t, fake.suspendCalls)
		})
	}
}

func TestBillingConsumer_UnknownEventType_NoOp(t *testing.T) {
	fake := &fakeSuspender{
		getTenantFn: func(context.Context, uuid.UUID) (*Tenant, error) {
			t.Fatal("GetTenant must not be called for an unknown event type")
			return nil, nil
		},
		suspendFn: func(context.Context, uuid.UUID, *time.Time, *string) (*Tenant, error) {
			t.Fatal("SuspendTenant must not be called for an unknown event type")
			return nil, nil
		},
	}
	h := NewBillingConsumer(fake)
	env := &events.Envelope{Type: "thittam.billing.subscription.reactivated", TenantID: uuid.New()}

	err := h.Handle(context.Background(), env)

	require.NoError(t, err)
	require.Empty(t, fake.suspendCalls)
}

func TestBillingConsumer_GetTenantError_ReturnsError(t *testing.T) {
	wantErr := errors.New("infra: connection reset")
	fake := &fakeSuspender{
		getTenantFn: func(context.Context, uuid.UUID) (*Tenant, error) {
			return nil, wantErr
		},
		suspendFn: func(context.Context, uuid.UUID, *time.Time, *string) (*Tenant, error) {
			t.Fatal("SuspendTenant must not be called when GetTenant fails")
			return nil, nil
		},
	}
	h := NewBillingConsumer(fake)
	env := &events.Envelope{Type: events.SubjectBillingSubscriptionSuspended, TenantID: uuid.New()}

	err := h.Handle(context.Background(), env)

	require.ErrorIs(t, err, wantErr)
	require.Empty(t, fake.suspendCalls)
}

func TestBillingConsumer_SuspendTenantError_ReturnsError(t *testing.T) {
	wantErr := errors.New("infra: deadlock detected")
	fake := &fakeSuspender{
		getTenantFn: tenantWithStatus(TenantStatusActive),
		suspendFn: func(context.Context, uuid.UUID, *time.Time, *string) (*Tenant, error) {
			return nil, wantErr
		},
	}
	h := NewBillingConsumer(fake)
	env := &events.Envelope{Type: events.SubjectBillingSubscriptionSuspended, TenantID: uuid.New()}

	err := h.Handle(context.Background(), env)

	require.ErrorIs(t, err, wantErr)
	require.Len(t, fake.suspendCalls, 1)
}

var _ tenantSuspender = (*fakeSuspender)(nil)
