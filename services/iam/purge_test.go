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

func newTestAuditLogger(store *memoryAuditStore) *audit.Logger {
	return audit.NewLogger(store, audit.LoggerConfig{
		BufferSize:    10,
		FlushInterval: 10 * time.Millisecond,
		BatchSize:     10,
	}, nil)
}

// --- RequestTenantPurge ---

func TestRequestTenantPurge_RejectsNonPurgeEligible(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusDeactivated}, nil
		},
		createTenantPurgeRequestFn: func(_ context.Context, _ *TenantPurgeRequest) error {
			t.Fatal("must not create a request for a non-purge_eligible tenant")
			return nil
		},
	}
	_, err := newTestService(repo).RequestTenantPurge(context.Background(), fixedTenantID, "gdpr erasure")
	assert.ErrorIs(t, err, ErrTenantNotPurgeable)
}

func TestRequestTenantPurge_DuplicateOpenPropagatesSentinel(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusPurgeEligible}, nil
		},
		createTenantPurgeRequestFn: func(_ context.Context, _ *TenantPurgeRequest) error {
			return ErrPurgeRequestExists
		},
	}
	_, err := newTestService(repo).RequestTenantPurge(context.Background(), fixedTenantID, "gdpr erasure")
	assert.ErrorIs(t, err, ErrPurgeRequestExists)
}

func TestRequestTenantPurge_Happy(t *testing.T) {
	t.Parallel()
	tenant := &Tenant{ID: fixedTenantID, Name: "Acme Films", Slug: "acme-films", Status: TenantStatusPurgeEligible}
	var created *TenantPurgeRequest
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return tenant, nil
		},
		createTenantPurgeRequestFn: func(_ context.Context, req *TenantPurgeRequest) error {
			created = req
			return nil
		},
	}
	store := &memoryAuditStore{}
	logger := newTestAuditLogger(store)
	svc := newTestService(repo).WithAuditLogger(logger)

	actorID := uuid.New()
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: actorID, Email: "ops@acme.com", IP: "10.0.0.5"})

	got, err := svc.RequestTenantPurge(ctx, fixedTenantID, "gdpr erasure")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, fixedTenantID, got.TenantID)
	assert.Equal(t, PurgeRequestPending, got.Status)
	assert.Equal(t, actorID, got.RequestedBy)
	assert.Equal(t, "gdpr erasure", got.RequestReason)
	assert.Equal(t, "Acme Films", got.TenantName)
	assert.Equal(t, "acme-films", got.TenantSlug)
	assert.Same(t, created, got)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, audit.ActionPurgeRequested, e.Action)
	assert.Equal(t, fixedTenantID, e.TenantID)
	assert.Equal(t, actorID, e.ActorID)
	assert.Equal(t, "ops@acme.com", e.ActorEmail)
	assert.JSONEq(t, `{"reason":"gdpr erasure"}`, string(e.Metadata))
}

// --- ApproveTenantPurge ---

func TestApproveTenantPurge_RejectsSelfApproval(t *testing.T) {
	t.Parallel()
	requester := uuid.New()
	repo := &mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, tid uuid.UUID) (*TenantPurgeRequest, error) {
			return &TenantPurgeRequest{ID: uuid.New(), TenantID: tid, Status: PurgeRequestPending, RequestedBy: requester}, nil
		},
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusPurgeEligible}, nil
		},
		approveTenantPurgeRequestFn: func(_ context.Context, _, _ uuid.UUID) (*TenantPurgeRequest, error) {
			t.Fatal("must not approve when approver == requester")
			return nil, nil
		},
	}
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: requester, Email: "a@b.c"})
	_, err := newTestService(repo).ApproveTenantPurge(ctx, fixedTenantID, "ok")
	assert.ErrorIs(t, err, ErrSelfApproval)
}

func TestApproveTenantPurge_RejectsWhenTenantLeftPurgeEligible(t *testing.T) {
	t.Parallel()
	requester := uuid.New()
	approver := uuid.New()
	repo := &mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, tid uuid.UUID) (*TenantPurgeRequest, error) {
			return &TenantPurgeRequest{ID: uuid.New(), TenantID: tid, Status: PurgeRequestPending, RequestedBy: requester}, nil
		},
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			// Tenant status changed (e.g. reactivated) between request and approval.
			return &Tenant{ID: id, Status: TenantStatusDeactivated}, nil
		},
		approveTenantPurgeRequestFn: func(_ context.Context, _, _ uuid.UUID) (*TenantPurgeRequest, error) {
			t.Fatal("must not approve when tenant is no longer purge_eligible")
			return nil, nil
		},
	}
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: approver, Email: "b@b.c"})
	_, err := newTestService(repo).ApproveTenantPurge(ctx, fixedTenantID, "ok")
	assert.ErrorIs(t, err, ErrTenantNotPurgeable)
}

func TestApproveTenantPurge_RejectsAlreadyProcessedRequest(t *testing.T) {
	t.Parallel()
	requester := uuid.New()
	approver := uuid.New()
	repo := &mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, tid uuid.UUID) (*TenantPurgeRequest, error) {
			return &TenantPurgeRequest{ID: uuid.New(), TenantID: tid, Status: PurgeRequestApproved, RequestedBy: requester}, nil
		},
		approveTenantPurgeRequestFn: func(_ context.Context, _, _ uuid.UUID) (*TenantPurgeRequest, error) {
			t.Fatal("must not re-approve a request that is not pending")
			return nil, nil
		},
	}
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: approver, Email: "b@b.c"})
	_, err := newTestService(repo).ApproveTenantPurge(ctx, fixedTenantID, "ok")
	assert.ErrorIs(t, err, ErrPurgeRequestNotFound)
}

func TestApproveTenantPurge_NoOpenRequest(t *testing.T) {
	t.Parallel()
	// mockRepo default (no getOpenTenantPurgeRequestFn) returns ErrPurgeRequestNotFound.
	repo := &mockRepo{}
	_, err := newTestService(repo).ApproveTenantPurge(context.Background(), fixedTenantID, "ok")
	assert.ErrorIs(t, err, ErrPurgeRequestNotFound)
}

func TestApproveTenantPurge_Happy(t *testing.T) {
	t.Parallel()
	requester := uuid.New()
	approver := uuid.New()
	openReq := &TenantPurgeRequest{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestPending, RequestedBy: requester}
	approved := &TenantPurgeRequest{ID: openReq.ID, TenantID: fixedTenantID, Status: PurgeRequestApproved, RequestedBy: requester, ApprovedBy: &approver}

	var gotRequestID, gotApproverID uuid.UUID
	repo := &mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, tid uuid.UUID) (*TenantPurgeRequest, error) {
			return openReq, nil
		},
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusPurgeEligible}, nil
		},
		approveTenantPurgeRequestFn: func(_ context.Context, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error) {
			gotRequestID, gotApproverID = requestID, approverID
			return approved, nil
		},
	}
	store := &memoryAuditStore{}
	logger := newTestAuditLogger(store)
	svc := newTestService(repo).WithAuditLogger(logger)

	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: approver, Email: "approver@acme.com"})
	got, err := svc.ApproveTenantPurge(ctx, fixedTenantID, "confirmed with legal")
	require.NoError(t, err)
	assert.Same(t, approved, got)
	assert.Equal(t, openReq.ID, gotRequestID)
	assert.Equal(t, approver, gotApproverID)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.ActionPurgeApproved, events[0].Action)
	assert.Equal(t, fixedTenantID, events[0].TenantID)
	assert.Equal(t, approver, events[0].ActorID)
	assert.JSONEq(t, `{"reason":"confirmed with legal"}`, string(events[0].Metadata))
}

// --- CancelTenantPurge ---

func TestCancelTenantPurge_Happy(t *testing.T) {
	t.Parallel()
	canceller := uuid.New()
	openReq := &TenantPurgeRequest{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestPending}
	cancelled := &TenantPurgeRequest{ID: openReq.ID, TenantID: fixedTenantID, Status: PurgeRequestCancelled}

	var gotRequestID, gotCancellerID uuid.UUID
	repo := &mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, tid uuid.UUID) (*TenantPurgeRequest, error) {
			return openReq, nil
		},
		cancelTenantPurgeRequestFn: func(_ context.Context, requestID, cancellerID uuid.UUID) (*TenantPurgeRequest, error) {
			gotRequestID, gotCancellerID = requestID, cancellerID
			return cancelled, nil
		},
	}
	store := &memoryAuditStore{}
	logger := newTestAuditLogger(store)
	svc := newTestService(repo).WithAuditLogger(logger)

	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: canceller, Email: "ops@acme.com"})
	got, err := svc.CancelTenantPurge(ctx, fixedTenantID, "customer reinstated")
	require.NoError(t, err)
	assert.Same(t, cancelled, got)
	assert.Equal(t, openReq.ID, gotRequestID)
	assert.Equal(t, canceller, gotCancellerID)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.ActionPurgeCancelled, events[0].Action)
	assert.Equal(t, fixedTenantID, events[0].TenantID)
}

func TestCancelTenantPurge_NoOpenRequest(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{}
	_, err := newTestService(repo).CancelTenantPurge(context.Background(), fixedTenantID, "n/a")
	assert.ErrorIs(t, err, ErrPurgeRequestNotFound)
}

// --- PurgeApprovedTenant (worker-facing) ---

func TestPurgeApprovedTenant_Success_EmitsAudit(t *testing.T) {
	t.Parallel()
	req := &TenantPurgeRequest{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestApproved, TenantName: "Doomed"}
	repo := &mockRepo{
		purgeTenantSchemaAndTombstoneFn: func(_ context.Context, _, _ uuid.UUID) error { return nil },
	}
	store := &memoryAuditStore{}
	logger := newTestAuditLogger(store)
	svc := newTestService(repo).WithAuditLogger(logger)

	err := svc.PurgeApprovedTenant(context.Background(), req)
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))
	events := store.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.ActionTenantPurged, events[0].Action)
	assert.Equal(t, fixedTenantID, events[0].TenantID)
	assert.Equal(t, SystemActorPurgeWorker, events[0].ActorEmail)
}

// TestPurgeApprovedTenant_Failure_MarksFailed is the load-bearing coverage
// note from the Task 3 review: when PurgeTenantSchemaAndTombstone races
// against the tenant status (e.g. someone cleared purge_eligible after
// approval) and returns ErrTenantNotPurgeable, PurgeApprovedTenant MUST:
//  1. NOT emit a success (ActionTenantPurged) audit event,
//  2. call MarkTenantPurgeRequestFailed with a non-empty reason so the
//     request is recorded as failed for operator visibility / retry triage,
//  3. return a non-nil error so the worker's caller aborts the run instead
//     of treating a failed purge as done.
func TestPurgeApprovedTenant_Failure_MarksFailed(t *testing.T) {
	t.Parallel()
	req := &TenantPurgeRequest{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestApproved}
	var markedRequestID uuid.UUID
	var markedReason string
	var markFailedCalled bool
	repo := &mockRepo{
		purgeTenantSchemaAndTombstoneFn: func(_ context.Context, _, _ uuid.UUID) error { return ErrTenantNotPurgeable },
		markTenantPurgeRequestFailedFn: func(_ context.Context, requestID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
			markFailedCalled = true
			markedRequestID = requestID
			markedReason = reason
			return req, nil
		},
	}
	store := &memoryAuditStore{}
	logger := newTestAuditLogger(store)
	svc := newTestService(repo).WithAuditLogger(logger)

	err := svc.PurgeApprovedTenant(context.Background(), req)
	require.Error(t, err, "PurgeApprovedTenant must return an error on purge failure")
	assert.ErrorIs(t, err, ErrTenantNotPurgeable)

	require.True(t, markFailedCalled, "MarkTenantPurgeRequestFailed must be called on failure")
	assert.Equal(t, req.ID, markedRequestID)
	assert.NotEmpty(t, markedReason, "failure reason must be recorded on the request")

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))
	events := store.snapshot()
	assert.Empty(t, events, "must not emit a success audit event on purge failure")
}

func TestPurgeApprovedTenant_MarkFailedErrors_WrapsBothErrors(t *testing.T) {
	t.Parallel()
	req := &TenantPurgeRequest{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestApproved}
	markFailedErr := assert.AnError
	repo := &mockRepo{
		purgeTenantSchemaAndTombstoneFn: func(_ context.Context, _, _ uuid.UUID) error { return ErrTenantNotPurgeable },
		markTenantPurgeRequestFailedFn: func(_ context.Context, _ uuid.UUID, _ string) (*TenantPurgeRequest, error) {
			return nil, markFailedErr
		},
	}
	svc := newTestService(repo)
	err := svc.PurgeApprovedTenant(context.Background(), req)
	require.Error(t, err)
	assert.ErrorIs(t, err, markFailedErr)
}

// --- ListApprovedPurges (worker-facing passthrough) ---

func TestListApprovedPurges_DelegatesToRepo(t *testing.T) {
	t.Parallel()
	want := []*TenantPurgeRequest{{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestApproved}}
	var gotLimit int
	repo := &mockRepo{
		listApprovedTenantPurgeRequestsFn: func(_ context.Context, limit int) ([]*TenantPurgeRequest, error) {
			gotLimit = limit
			return want, nil
		},
	}
	got, err := newTestService(repo).ListApprovedPurges(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 100, gotLimit)
}
