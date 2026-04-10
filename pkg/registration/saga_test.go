package registration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Saga mocks ---

// mockSagaStore is an in-memory SagaStore for tests.
type mockSagaStore struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*RegistrationSaga
	byEmail map[string]*RegistrationSaga

	createErr error
	updateErr error
}

func newMockSagaStore() *mockSagaStore {
	return &mockSagaStore{
		byID:    make(map[uuid.UUID]*RegistrationSaga),
		byEmail: make(map[string]*RegistrationSaga),
	}
}

func (m *mockSagaStore) Create(ctx context.Context, saga *RegistrationSaga) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// deep copy so mutations don't affect stored state unexpectedly
	copy := *saga
	m.byID[saga.ID] = &copy
	m.byEmail[saga.Email] = &copy
	return nil
}

func (m *mockSagaStore) Update(ctx context.Context, saga *RegistrationSaga) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *saga
	m.byID[saga.ID] = &copy
	m.byEmail[saga.Email] = &copy
	return nil
}

func (m *mockSagaStore) GetByID(ctx context.Context, id uuid.UUID) (*RegistrationSaga, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[id]
	if !ok {
		return nil, ErrSagaNotFound
	}
	copy := *s
	return &copy, nil
}

func (m *mockSagaStore) GetByEmail(ctx context.Context, email string) (*RegistrationSaga, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byEmail[email]
	if !ok {
		return nil, ErrSagaNotFound
	}
	copy := *s
	return &copy, nil
}

// mockCompensator records which compensating actions were called.
type mockCompensator struct {
	deletedTenants  []uuid.UUID
	deletedUsers    []uuid.UUID
	unboundVerticals []string

	deleteTenantErr  error
	deleteUserErr    error
	unbindVerticalErr error
}

func (m *mockCompensator) DeleteTenant(ctx context.Context, tenantID uuid.UUID) error {
	m.deletedTenants = append(m.deletedTenants, tenantID)
	return m.deleteTenantErr
}

func (m *mockCompensator) DeleteUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	m.deletedUsers = append(m.deletedUsers, userID)
	return m.deleteUserErr
}

func (m *mockCompensator) UnbindVertical(ctx context.Context, tenantID uuid.UUID, verticalID string) error {
	m.unboundVerticals = append(m.unboundVerticals, verticalID)
	return m.unbindVerticalErr
}

// --- Orchestrator test helpers ---

func newTestOrchestrator(pipeline *Pipeline, store SagaStore, comp Compensator) *Orchestrator {
	return NewOrchestrator(pipeline, store, comp, mockLogger{})
}

// --- Tests ---

func TestOrchestrator_HappyPath(t *testing.T) {
	t.Parallel()

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(newTestPipeline(), store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.NoError(t, err)
	require.NotNil(t, saga)

	assert.Equal(t, SagaCompleted, saga.Status)
	assert.NotEqual(t, uuid.Nil, saga.TenantID)
	assert.NotEqual(t, uuid.Nil, saga.UserID)
	assert.Len(t, saga.CompletedSteps, 9)
	assert.Empty(t, comp.deletedTenants, "no compensation should run on success")
}

func TestOrchestrator_FailAtStep1_NoCompensation(t *testing.T) {
	t.Parallel()

	// Step 1 (CreateTenant) fails before anything is committed, so no compensation.
	dbErr := errors.New("postgres down")
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			createTenantFn: func(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
				return uuid.Nil, dbErr
			},
		}
	})

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(p, store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.Error(t, err)
	require.NotNil(t, saga)

	// failSaga (not failAndCompensate) because StepCreateTenant never completed.
	assert.Equal(t, SagaFailed, saga.Status)
	assert.Equal(t, StepCreateTenant, saga.FailedStep)
	assert.Empty(t, comp.deletedTenants)
	assert.Empty(t, comp.deletedUsers)
}

func TestOrchestrator_FailAtStep2_CompensatesStep1(t *testing.T) {
	t.Parallel()

	// Step 1 succeeds, step 2 (CreateUser) fails → compensate: DeleteTenant.
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			createUserFn: func(ctx context.Context, tenantID uuid.UUID, email, name, hash string) (uuid.UUID, error) {
				return uuid.Nil, errors.New("user create failed")
			},
		}
	})

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(p, store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.Error(t, err)
	require.NotNil(t, saga)

	assert.Equal(t, SagaCompensated, saga.Status)
	assert.Equal(t, StepCreateUser, saga.FailedStep)
	assert.Len(t, comp.deletedTenants, 1, "DeleteTenant must run")
	assert.Empty(t, comp.deletedUsers, "CreateUser failed so no user to delete")
	assert.Empty(t, comp.unboundVerticals)
}

func TestOrchestrator_FailAtStep3_CompensatesSteps2And1(t *testing.T) {
	t.Parallel()

	// Steps 1-2 succeed, step 3 (BindVertical) fails → compensate: DeleteUser, DeleteTenant.
	p := newTestPipeline(func(p *Pipeline) {
		p.verticals = &mockVerticalStore{
			bindTenantVerticalFn: func(ctx context.Context, tenantID uuid.UUID, verticalID string, by uuid.UUID) error {
				return errors.New("vertical bind failed")
			},
		}
	})

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(p, store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.Error(t, err)
	require.NotNil(t, saga)

	assert.Equal(t, SagaCompensated, saga.Status)
	assert.Equal(t, StepBindVertical, saga.FailedStep)
	assert.Empty(t, comp.unboundVerticals, "BindVertical failed so no binding to undo")
	assert.Len(t, comp.deletedUsers, 1, "DeleteUser must run")
	assert.Len(t, comp.deletedTenants, 1, "DeleteTenant must run")
}

func TestOrchestrator_FailAtStep4_CompensatesSteps3to1(t *testing.T) {
	t.Parallel()

	// Steps 1-3 succeed, step 4 (SeedChartOfAccounts) fails.
	// Compensation: UnbindVertical → DeleteUser → DeleteTenant.
	p := newTestPipeline(func(p *Pipeline) {
		p.ledger = &mockLedgerSeeder{
			seedCoAFn: func(ctx context.Context, tenantID uuid.UUID, _ []vertical.ChartOfAccountEntry) error {
				return errors.New("ledger unavailable")
			},
		}
	})

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(p, store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.Error(t, err)
	require.NotNil(t, saga)

	assert.Equal(t, SagaCompensated, saga.Status)
	assert.Equal(t, StepSeedChartOfAccounts, saga.FailedStep)
	// All three compensating actions must have run.
	assert.Len(t, comp.unboundVerticals, 1, "UnbindVertical must run")
	assert.Len(t, comp.deletedUsers, 1, "DeleteUser must run")
	assert.Len(t, comp.deletedTenants, 1, "DeleteTenant must run")
}

func TestOrchestrator_BestEffortFailure_SagaStillCompleted(t *testing.T) {
	t.Parallel()

	// Steps 8 and 9 fail but saga must end as SagaCompleted.
	p := newTestPipeline(func(p *Pipeline) {
		p.events = &mockEventPublisher{
			publishFn: func(ctx context.Context, ev TenantCreatedEvent) error {
				return errors.New("NATS partition")
			},
		}
		p.notifier = &mockNotifier{
			sendFn: func(ctx context.Context, tenantID uuid.UUID, email, company, vertical string) error {
				return errors.New("SMTP down")
			},
		}
	})

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(p, store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.NoError(t, err, "best-effort failures must not propagate")
	require.NotNil(t, saga)

	assert.Equal(t, SagaCompleted, saga.Status)
	assert.Empty(t, comp.deletedTenants, "no compensation on best-effort failure")
}

func TestOrchestrator_CompensationFailure_RecordsErrors(t *testing.T) {
	t.Parallel()

	// Step 3 fails, and DeleteTenant also fails during compensation.
	p := newTestPipeline(func(p *Pipeline) {
		p.verticals = &mockVerticalStore{
			bindTenantVerticalFn: func(ctx context.Context, tenantID uuid.UUID, verticalID string, by uuid.UUID) error {
				return errors.New("bind failed")
			},
		}
	})

	store := newMockSagaStore()
	comp := &mockCompensator{
		deleteTenantErr: errors.New("DeleteTenant: network timeout"),
	}
	orch := newTestOrchestrator(p, store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.Error(t, err)
	require.NotNil(t, saga)

	assert.Equal(t, SagaCompensationFailed, saga.Status)
	require.NotEmpty(t, saga.CompensationErrors)
	assert.Contains(t, saga.CompensationErrors[0], "DeleteTenant")
}

func TestOrchestrator_EmailDedup_ReturnsExistingInProgressSaga(t *testing.T) {
	t.Parallel()

	store := newMockSagaStore()
	comp := &mockCompensator{}

	// First start puts a completed saga in the store.
	orch := newTestOrchestrator(newTestPipeline(), store, comp)
	first, err := orch.Start(context.Background(), testRequest())
	require.NoError(t, err)
	require.Equal(t, SagaCompleted, first.Status)

	// Second start with the same email returns the existing saga.
	second, err := orch.Start(context.Background(), testRequest())
	require.NoError(t, err, "completed saga should return without error")
	assert.Equal(t, first.ID, second.ID)
}

func TestOrchestrator_Resume_SkipsCompletedSteps(t *testing.T) {
	t.Parallel()

	// Fail at step 4 so steps 1-3 are completed.
	seedCallCount := 0
	p := newTestPipeline(func(p *Pipeline) {
		p.ledger = &mockLedgerSeeder{
			seedCoAFn: func(ctx context.Context, tenantID uuid.UUID, _ []vertical.ChartOfAccountEntry) error {
				seedCallCount++
				if seedCallCount == 1 {
					return errors.New("temporary ledger failure")
				}
				return nil
			},
		}
	})

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(p, store, comp)

	// First attempt: fails at step 4, compensates, saga is SagaCompensated.
	saga, err := orch.Start(context.Background(), testRequest())
	require.Error(t, err)
	require.NotNil(t, saga)
	assert.Equal(t, SagaCompensated, saga.Status)

	// Resume is only valid on SagaFailed — so the store mock must hold a SagaFailed record.
	// Manually inject a failed saga to test resume logic.
	failedSagaID := uuid.MustParse("d1000000-0000-0000-0000-000000000001")
	failedSaga := &RegistrationSaga{
		ID:     failedSagaID,
		Email:  "resume@test.com",
		Status: SagaFailed,
		CompletedSteps: []Step{StepCreateTenant, StepCreateUser, StepBindVertical},
		FailedStep:     StepSeedChartOfAccounts,
		Request:        testRequest(),
		TenantID:       uuid.MustParse("d2000000-0000-0000-0000-000000000001"),
		UserID:         uuid.MustParse("d3000000-0000-0000-0000-000000000001"),
	}
	failedSaga.Request.Email = "resume@test.com"
	require.NoError(t, store.Create(context.Background(), failedSaga))

	// Second call: the ledger will succeed (seedCallCount == 2).
	resumed, err := orch.Resume(context.Background(), failedSagaID)
	require.NoError(t, err)
	require.NotNil(t, resumed)

	assert.Equal(t, SagaCompleted, resumed.Status)
	// Steps 1-3 were already completed; only steps 4-9 should have run.
	// The TenantID must be preserved from the previous run.
	assert.Equal(t, failedSaga.TenantID, resumed.TenantID)
}

func TestOrchestrator_Resume_RejectsCompletedSaga(t *testing.T) {
	t.Parallel()

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(newTestPipeline(), store, comp)

	saga, err := orch.Start(context.Background(), testRequest())
	require.NoError(t, err)
	require.Equal(t, SagaCompleted, saga.Status)

	// Resume on a completed saga returns it without error.
	resumed, err := orch.Resume(context.Background(), saga.ID)
	require.NoError(t, err)
	assert.Equal(t, SagaCompleted, resumed.Status)
}

func TestOrchestrator_Resume_RejectsInProgressSaga(t *testing.T) {
	t.Parallel()

	store := newMockSagaStore()
	inProgress := &RegistrationSaga{
		ID:      uuid.MustParse("d1000000-0000-0000-0000-000000000002"),
		Email:   "inprogress@test.com",
		Status:  SagaInProgress,
		Request: testRequest(),
	}
	require.NoError(t, store.Create(context.Background(), inProgress))

	comp := &mockCompensator{}
	orch := newTestOrchestrator(newTestPipeline(), store, comp)

	_, err := orch.Resume(context.Background(), inProgress.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be resumed")
}

func TestOrchestrator_GetStatus_ReturnsSagaState(t *testing.T) {
	t.Parallel()

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(newTestPipeline(), store, comp)

	created, err := orch.Start(context.Background(), testRequest())
	require.NoError(t, err)

	fetched, err := orch.GetStatus(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, SagaCompleted, fetched.Status)
}

func TestOrchestrator_GetStatus_ErrSagaNotFound(t *testing.T) {
	t.Parallel()

	store := newMockSagaStore()
	comp := &mockCompensator{}
	orch := newTestOrchestrator(newTestPipeline(), store, comp)

	_, err := orch.GetStatus(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSagaNotFound)
}
