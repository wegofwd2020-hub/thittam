package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/events"
)

// ── Stub mocks ───────────────────────────────────────────────────────────────

type stubMessage struct {
	subject string
	data    []byte
	acked   bool
	naked   bool
}

func (m *stubMessage) Subject() string { return m.subject }
func (m *stubMessage) Data() []byte    { return m.data }
func (m *stubMessage) Ack() error      { m.acked = true; return nil }
func (m *stubMessage) Nak() error      { m.naked = true; return nil }

func mustMarshalEnvelope(t *testing.T, subject string, tenantID uuid.UUID, occurredAt time.Time, payload interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	env := events.Envelope{
		EventID:    uuid.MustParse("e1000000-0000-0000-0000-000000000001"),
		Type:       subject,
		TenantID:   tenantID,
		OccurredAt: occurredAt,
		Payload:    raw,
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)
	return data
}

// stubProjectionWriter records every write call for assertions.
type stubProjectionWriter struct {
	mu sync.Mutex

	expenseFacts       []ExpenseFact
	monthlySpendCalls  []struct{ TenantID uuid.UUID; Month string; Delta decimal.Decimal }
	categorySpendCalls []struct{ TenantID uuid.UUID; CategoryID string; Delta decimal.Decimal }
	pendingUpserts     []PendingApprovalRow
	pendingDeletes     []uuid.UUID
	budgetFacts        []BudgetFact
	portfolioSnapshots []PortfolioSnapshotRow
	teamAssignments    []struct{ TenantID, ProjectID uuid.UUID; Count int }

	// Injected error overrides for failure tests.
	upsertExpenseErr    error
	upsertBudgetErr     error
	upsertSnapshotErr   error
	upsertPendingErr    error
	incrementMonthlyErr error
	incrementCategoryErr error
	deletePendingErr    error
	upsertTeamErr       error
}

func (s *stubProjectionWriter) UpsertExpenseFact(_ context.Context, f ExpenseFact) error {
	if s.upsertExpenseErr != nil {
		return s.upsertExpenseErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expenseFacts = append(s.expenseFacts, f)
	return nil
}

func (s *stubProjectionWriter) IncrementMonthlySpend(_ context.Context, tenantID uuid.UUID, month string, delta decimal.Decimal) error {
	if s.incrementMonthlyErr != nil {
		return s.incrementMonthlyErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.monthlySpendCalls = append(s.monthlySpendCalls, struct {
		TenantID uuid.UUID
		Month    string
		Delta    decimal.Decimal
	}{tenantID, month, delta})
	return nil
}

func (s *stubProjectionWriter) IncrementCategorySpend(_ context.Context, tenantID uuid.UUID, categoryID, _ string, delta decimal.Decimal) error {
	if s.incrementCategoryErr != nil {
		return s.incrementCategoryErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.categorySpendCalls = append(s.categorySpendCalls, struct {
		TenantID   uuid.UUID
		CategoryID string
		Delta      decimal.Decimal
	}{tenantID, categoryID, delta})
	return nil
}

func (s *stubProjectionWriter) UpsertPendingApproval(_ context.Context, item PendingApprovalRow) error {
	if s.upsertPendingErr != nil {
		return s.upsertPendingErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingUpserts = append(s.pendingUpserts, item)
	return nil
}

func (s *stubProjectionWriter) DeletePendingApproval(_ context.Context, _, expenseID uuid.UUID) error {
	if s.deletePendingErr != nil {
		return s.deletePendingErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingDeletes = append(s.pendingDeletes, expenseID)
	return nil
}

func (s *stubProjectionWriter) UpsertBudgetFact(_ context.Context, f BudgetFact) error {
	if s.upsertBudgetErr != nil {
		return s.upsertBudgetErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.budgetFacts = append(s.budgetFacts, f)
	return nil
}

func (s *stubProjectionWriter) UpsertPortfolioSnapshot(_ context.Context, snap PortfolioSnapshotRow) error {
	if s.upsertSnapshotErr != nil {
		return s.upsertSnapshotErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.portfolioSnapshots = append(s.portfolioSnapshots, snap)
	return nil
}

func (s *stubProjectionWriter) UpsertTeamAssignment(_ context.Context, tenantID, projectID uuid.UUID, _ string, memberCount int) error {
	if s.upsertTeamErr != nil {
		return s.upsertTeamErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teamAssignments = append(s.teamAssignments, struct {
		TenantID, ProjectID uuid.UUID
		Count               int
	}{tenantID, projectID, memberCount})
	return nil
}

// stubWatermarkStore is an in-memory WatermarkStore.
type stubWatermarkStore struct {
	mu      sync.Mutex
	marks   map[string]time.Time
	getErr  error
}

func newStubWatermarkStore() *stubWatermarkStore {
	return &stubWatermarkStore{marks: make(map[string]time.Time)}
}

func (s *stubWatermarkStore) RecordWatermark(_ context.Context, subject string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.marks[subject]; !ok || t.After(existing) {
		s.marks[subject] = t
	}
	return nil
}

func (s *stubWatermarkStore) OldestWatermark(_ context.Context) (time.Time, error) {
	if s.getErr != nil {
		return time.Time{}, s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var oldest time.Time
	for _, t := range s.marks {
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	return oldest, nil
}

type noopLogger struct{}

func (noopLogger) Info(string, ...interface{})  {}
func (noopLogger) Warn(string, ...interface{})  {}
func (noopLogger) Error(string, ...interface{}) {}

func newTestConsumer(writer ProjectionWriter, wm WatermarkStore) *ProjectionConsumer {
	return NewProjectionConsumer(writer, wm, noopLogger{})
}

// ── Handler tests ─────────────────────────────────────────────────────────────

func TestConsumer_HandleExpenseSubmitted_UpsertsPendingApproval(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000001")
	expenseID := uuid.MustParse("d2000000-0000-0000-0000-000000000001")
	submittedAt := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	occurredAt := submittedAt

	payload := events.ExpenseSubmittedPayload{
		ExpenseID:    expenseID,
		ProductionID: uuid.MustParse("d3000000-0000-0000-0000-000000000001"),
		CategoryID:   "salary",
		Amount:       "50000.00",
		SubmittedBy:  "Alice",
		SubmittedAt:  submittedAt,
		Description:  "Monthly salary",
		ProjectTitle: "Ad Campaign Q1",
	}

	writer := &stubProjectionWriter{}
	wm := newStubWatermarkStore()
	consumer := newTestConsumer(writer, wm)

	msg := &stubMessage{
		subject: events.SubjectExpenseSubmitted,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseSubmitted, tenantID, occurredAt, payload),
	}

	err := consumer.handleExpenseSubmitted(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, msg.acked)
	assert.False(t, msg.naked)

	require.Len(t, writer.pendingUpserts, 1)
	row := writer.pendingUpserts[0]
	assert.Equal(t, expenseID, row.ID)
	assert.Equal(t, tenantID, row.TenantID)
	assert.Equal(t, "expense", row.ItemType)
	assert.Equal(t, decimal.RequireFromString("50000.00"), row.Amount)
	assert.Equal(t, "Alice", row.SubmittedBy)

	// Watermark must be recorded.
	oldest, _ := wm.OldestWatermark(context.Background())
	assert.Equal(t, occurredAt.UTC(), oldest.UTC())
}

func TestConsumer_HandleExpenseApproved_UpdatesAllProjections(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000002")
	expenseID := uuid.MustParse("d2000000-0000-0000-0000-000000000002")
	approvedAt := time.Date(2026, 4, 10, 11, 0, 0, 0, time.UTC)

	payload := events.ExpenseApprovedPayload{
		ExpenseID:    expenseID,
		ProductionID: uuid.MustParse("d3000000-0000-0000-0000-000000000002"),
		CategoryID:   "salary",
		CategoryName: "Salary / Payroll",
		Amount:       "50000.00",
		ApprovedAt:   approvedAt,
		Month:        "2026-04",
	}

	writer := &stubProjectionWriter{}
	wm := newStubWatermarkStore()
	consumer := newTestConsumer(writer, wm)

	msg := &stubMessage{
		subject: events.SubjectExpenseApproved,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseApproved, tenantID, approvedAt, payload),
	}

	err := consumer.handleExpenseApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, msg.acked)

	// expense_facts upsert
	require.Len(t, writer.expenseFacts, 1)
	assert.Equal(t, expenseID, writer.expenseFacts[0].ExpenseID)
	assert.Equal(t, decimal.RequireFromString("50000.00"), writer.expenseFacts[0].Amount)

	// monthly_spend increment
	require.Len(t, writer.monthlySpendCalls, 1)
	assert.Equal(t, "2026-04", writer.monthlySpendCalls[0].Month)
	assert.Equal(t, decimal.RequireFromString("50000.00"), writer.monthlySpendCalls[0].Delta)

	// category_spend increment
	require.Len(t, writer.categorySpendCalls, 1)
	assert.Equal(t, "salary", writer.categorySpendCalls[0].CategoryID)

	// pending_approval deleted
	require.Len(t, writer.pendingDeletes, 1)
	assert.Equal(t, expenseID, writer.pendingDeletes[0])
}

func TestConsumer_HandleBudgetApproved_UpsertsBudgetFact(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000003")
	budgetID := uuid.MustParse("d2000000-0000-0000-0000-000000000003")

	payload := events.BudgetApprovedPayload{
		BudgetID:       budgetID,
		ProductionID:   uuid.MustParse("d3000000-0000-0000-0000-000000000003"),
		TotalBudgeted:  "200000.00",
		TotalActual:    "50000.00",
		TotalCommitted: "30000.00",
	}

	writer := &stubProjectionWriter{}
	wm := newStubWatermarkStore()
	consumer := newTestConsumer(writer, wm)

	msg := &stubMessage{
		subject: events.SubjectBudgetApproved,
		data:    mustMarshalEnvelope(t, events.SubjectBudgetApproved, tenantID, time.Now(), payload),
	}

	err := consumer.handleBudgetApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, msg.acked)

	require.Len(t, writer.budgetFacts, 1)
	fact := writer.budgetFacts[0]
	assert.Equal(t, budgetID, fact.BudgetID)
	assert.Equal(t, decimal.RequireFromString("200000.00"), fact.TotalBudgeted)
	assert.Equal(t, decimal.RequireFromString("50000.00"), fact.TotalActual)
	assert.Equal(t, decimal.RequireFromString("30000.00"), fact.TotalCommitted)
}

func TestConsumer_HandleProjectCreated_UpsertsPortfolioSnapshot(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000004")
	projectID := uuid.MustParse("d2000000-0000-0000-0000-000000000004")
	startDate := "2026-04-01"

	payload := events.ProjectCreatedPayload{
		ProjectID: projectID,
		Title:     "Ad Campaign Q2",
		Status:    "active",
		StartDate: &startDate,
	}

	writer := &stubProjectionWriter{}
	wm := newStubWatermarkStore()
	consumer := newTestConsumer(writer, wm)

	msg := &stubMessage{
		subject: events.SubjectProjectCreated,
		data:    mustMarshalEnvelope(t, events.SubjectProjectCreated, tenantID, time.Now(), payload),
	}

	err := consumer.handleProjectCreated(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, msg.acked)

	require.Len(t, writer.portfolioSnapshots, 1)
	snap := writer.portfolioSnapshots[0]
	assert.Equal(t, projectID, snap.ProjectID)
	assert.Equal(t, "Ad Campaign Q2", snap.Title)
	assert.Equal(t, "active", snap.Status)
	require.NotNil(t, snap.StartDate)
	assert.Equal(t, "2026-04-01", *snap.StartDate)
}

func TestConsumer_HandleProjectStatusChanged_UpdatesSnapshot(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000005")
	projectID := uuid.MustParse("d2000000-0000-0000-0000-000000000005")

	payload := events.ProjectStatusChangedPayload{
		ProjectID:    projectID,
		Title:        "Ad Campaign Q2",
		OldStatus:    "active",
		NewStatus:    "completed",
		CurrentPhase: "Post-Production",
	}

	writer := &stubProjectionWriter{}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectProjectStatusChanged,
		data:    mustMarshalEnvelope(t, events.SubjectProjectStatusChanged, tenantID, time.Now(), payload),
	}

	err := consumer.handleProjectStatusChanged(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, msg.acked)

	require.Len(t, writer.portfolioSnapshots, 1)
	assert.Equal(t, "completed", writer.portfolioSnapshots[0].Status)
	assert.Equal(t, "Post-Production", writer.portfolioSnapshots[0].CurrentPhase)
}

func TestConsumer_HandleProjectMemberAssigned_UpsertsTeamAssignment(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000006")
	projectID := uuid.MustParse("d2000000-0000-0000-0000-000000000006")

	payload := events.ProjectMemberAssignedPayload{
		ProjectID:    projectID,
		ProjectTitle: "Feature Film",
		MemberCount:  12,
	}

	writer := &stubProjectionWriter{}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectProjectMemberAssigned,
		data:    mustMarshalEnvelope(t, events.SubjectProjectMemberAssigned, tenantID, time.Now(), payload),
	}

	err := consumer.handleProjectMemberAssigned(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, msg.acked)

	require.Len(t, writer.teamAssignments, 1)
	assert.Equal(t, projectID, writer.teamAssignments[0].ProjectID)
	assert.Equal(t, 12, writer.teamAssignments[0].Count)
}

func TestConsumer_BadEnvelope_AcksAndSkips(t *testing.T) {
	t.Parallel()

	writer := &stubProjectionWriter{}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectExpenseApproved,
		data:    []byte("not json"),
	}

	err := consumer.handleExpenseApproved(context.Background(), msg)
	require.NoError(t, err, "bad envelope must not return an error — message is acked and skipped")
	assert.True(t, msg.acked, "bad message must be acked to advance consumer cursor")
	assert.Empty(t, writer.expenseFacts, "no projection write should occur")
}

func TestConsumer_BadAmount_AcksAndSkips(t *testing.T) {
	t.Parallel()

	payload := events.ExpenseApprovedPayload{
		ExpenseID:  uuid.New(),
		Amount:     "not-a-number",
		ApprovedAt: time.Now(),
		Month:      "2026-04",
	}

	writer := &stubProjectionWriter{}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectExpenseApproved,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseApproved, uuid.New(), time.Now(), payload),
	}

	err := consumer.handleExpenseApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, msg.acked)
	assert.Empty(t, writer.expenseFacts)
}

func TestConsumer_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	// When the DB write fails, the handler returns nil (no panic) but NAKs the
	// message so NATS redelivers it after the DB recovers.
	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000007")
	payload := events.BudgetApprovedPayload{
		BudgetID:       uuid.New(),
		ProductionID:   uuid.New(),
		TotalBudgeted:  "100.00",
		TotalActual:    "0.00",
		TotalCommitted: "0.00",
	}

	writer := &stubProjectionWriter{upsertBudgetErr: errors.New("connection refused")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectBudgetApproved,
		data:    mustMarshalEnvelope(t, events.SubjectBudgetApproved, tenantID, time.Now(), payload),
	}

	err := consumer.handleBudgetApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error must NAK so NATS redelivers after recovery")
}

// ── Register tests ────────────────────────────────────────────────────────────

type trackingSubscriber struct {
	subscribed []string
}

func (s *trackingSubscriber) Subscribe(subject string, _ func(context.Context, Message) error) error {
	s.subscribed = append(s.subscribed, subject)
	return nil
}

func TestProjectionConsumer_Register_SubscribesToAllSubjects(t *testing.T) {
	t.Parallel()

	consumer := newTestConsumer(&stubProjectionWriter{}, newStubWatermarkStore())
	sub := &trackingSubscriber{}

	require.NoError(t, consumer.Register(sub))

	expected := []string{
		events.SubjectExpenseSubmitted,
		events.SubjectExpenseApproved,
		events.SubjectBudgetApproved,
		events.SubjectProjectCreated,
		events.SubjectProjectStatusChanged,
		events.SubjectProjectMemberAssigned,
	}
	assert.ElementsMatch(t, expected, sub.subscribed)
}

// ── LagChecker tests ──────────────────────────────────────────────────────────

func TestLagChecker_OK_WhenNoWatermarks(t *testing.T) {
	t.Parallel()

	// Cold start: no events have been processed yet. Should pass.
	wm := newStubWatermarkStore()
	checker := NewLagChecker(wm)

	assert.NoError(t, checker.CheckHealth(context.Background()))
}

func TestLagChecker_OK_WhenLagWithinSLA(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	wm := newStubWatermarkStore()
	// Watermark 10 seconds ago — within 30 s SLA.
	require.NoError(t, wm.RecordWatermark(context.Background(), events.SubjectExpenseApproved, now.Add(-10*time.Second)))

	checker := &LagChecker{watermark: wm, sla: ProjectionLagSLA, clock: func() time.Time { return now }}

	assert.NoError(t, checker.CheckHealth(context.Background()))
}

func TestLagChecker_Fails_WhenLagExceedsSLA(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	wm := newStubWatermarkStore()
	// Watermark 60 seconds ago — exceeds 30 s SLA.
	require.NoError(t, wm.RecordWatermark(context.Background(), events.SubjectExpenseApproved, now.Add(-60*time.Second)))

	checker := &LagChecker{watermark: wm, sla: ProjectionLagSLA, clock: func() time.Time { return now }}

	err := checker.CheckHealth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lag")
	assert.Contains(t, err.Error(), "SLA")
}

func TestLagChecker_Fails_WhenWatermarkStoreErrors(t *testing.T) {
	t.Parallel()

	wm := &stubWatermarkStore{getErr: errors.New("postgres down")}
	checker := NewLagChecker(wm)

	err := checker.CheckHealth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lag check")
}

func TestLagChecker_UsesOldestWatermarkAcrossSubjects(t *testing.T) {
	t.Parallel()

	// Two subjects: one fresh, one stale. Must flag based on the stale one.
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	wm := newStubWatermarkStore()
	require.NoError(t, wm.RecordWatermark(context.Background(), events.SubjectExpenseApproved, now.Add(-5*time.Second)))
	require.NoError(t, wm.RecordWatermark(context.Background(), events.SubjectProjectCreated, now.Add(-90*time.Second)))

	checker := &LagChecker{watermark: wm, sla: ProjectionLagSLA, clock: func() time.Time { return now }}

	err := checker.CheckHealth(context.Background())
	require.Error(t, err, "oldest watermark is 90s ago — should fail even though one subject is fresh")
}

// ── Additional consumer tests for error paths ────────────────────────────────

// TestDecodeEnvelope_MissingType ensures an envelope missing the type field is rejected.
func TestDecodeEnvelope_MissingType(t *testing.T) {
	t.Parallel()
	env := events.Envelope{
		EventID:    uuid.MustParse("e1000000-0000-0000-0000-000000000001"),
		OccurredAt: time.Now(),
		// Type intentionally empty
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	_, err = decodeEnvelope(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing type")
}

// TestDecodeEnvelope_MissingEventID ensures an envelope with nil event ID is rejected.
func TestDecodeEnvelope_MissingEventID(t *testing.T) {
	t.Parallel()
	env := events.Envelope{
		Type:       events.SubjectExpenseSubmitted,
		OccurredAt: time.Now(),
		// EventID intentionally zero
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	_, err = decodeEnvelope(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing event_id")
}

// TestAdvanceWatermark_WatermarkError tests that watermark errors are logged but not fatal.
func TestAdvanceWatermark_WatermarkErrorIsSilent(t *testing.T) {
	t.Parallel()
	failWM := &failingWatermarkStore{}
	consumer := newTestConsumer(&stubProjectionWriter{}, failWM)

	// Should not panic — error is absorbed.
	consumer.advanceWatermark(context.Background(), events.SubjectExpenseSubmitted, time.Now())
}

type failingWatermarkStore struct{}

func (*failingWatermarkStore) RecordWatermark(_ context.Context, _ string, _ time.Time) error {
	return errors.New("watermark store unavailable")
}
func (*failingWatermarkStore) OldestWatermark(_ context.Context) (time.Time, error) {
	return time.Time{}, nil
}

// ── NAK-path tests (DB error → message redelivery) ────────────────────────────

func TestConsumer_HandleExpenseSubmitted_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000010")
	payload := events.ExpenseSubmittedPayload{
		ExpenseID:    uuid.MustParse("d2000000-0000-0000-0000-000000000010"),
		ProductionID: uuid.MustParse("d3000000-0000-0000-0000-000000000010"),
		Amount:       "1000.00",
		SubmittedAt:  time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
	}

	writer := &stubProjectionWriter{upsertPendingErr: errors.New("db timeout")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectExpenseSubmitted,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseSubmitted, tenantID, time.Now(), payload),
	}

	err := consumer.handleExpenseSubmitted(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on UpsertPendingApproval must NAK for redelivery")
}

func TestConsumer_HandleExpenseApproved_UpsertFact_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000011")
	payload := events.ExpenseApprovedPayload{
		ExpenseID:    uuid.MustParse("d2000000-0000-0000-0000-000000000011"),
		ProductionID: uuid.MustParse("d3000000-0000-0000-0000-000000000011"),
		CategoryID:   "crew",
		Amount:       "2000.00",
		ApprovedAt:   time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
		Month:        "2026-04",
	}

	writer := &stubProjectionWriter{upsertExpenseErr: errors.New("unique violation")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectExpenseApproved,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseApproved, tenantID, time.Now(), payload),
	}

	err := consumer.handleExpenseApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on UpsertExpenseFact must NAK for redelivery")
}

func TestConsumer_HandleExpenseApproved_IncrementMonthly_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000012")
	payload := events.ExpenseApprovedPayload{
		ExpenseID:    uuid.MustParse("d2000000-0000-0000-0000-000000000012"),
		ProductionID: uuid.MustParse("d3000000-0000-0000-0000-000000000012"),
		CategoryID:   "equipment",
		Amount:       "3000.00",
		ApprovedAt:   time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
		Month:        "2026-04",
	}

	writer := &stubProjectionWriter{incrementMonthlyErr: errors.New("connection reset")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectExpenseApproved,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseApproved, tenantID, time.Now(), payload),
	}

	err := consumer.handleExpenseApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on IncrementMonthlySpend must NAK for redelivery")
}

func TestConsumer_HandleExpenseApproved_IncrementCategory_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000013")
	payload := events.ExpenseApprovedPayload{
		ExpenseID:    uuid.MustParse("d2000000-0000-0000-0000-000000000013"),
		ProductionID: uuid.MustParse("d3000000-0000-0000-0000-000000000013"),
		CategoryID:   "props",
		Amount:       "500.00",
		ApprovedAt:   time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
		Month:        "2026-04",
	}

	writer := &stubProjectionWriter{incrementCategoryErr: errors.New("lock timeout")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectExpenseApproved,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseApproved, tenantID, time.Now(), payload),
	}

	err := consumer.handleExpenseApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on IncrementCategorySpend must NAK for redelivery")
}

func TestConsumer_HandleExpenseApproved_DeletePending_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000014")
	payload := events.ExpenseApprovedPayload{
		ExpenseID:    uuid.MustParse("d2000000-0000-0000-0000-000000000014"),
		ProductionID: uuid.MustParse("d3000000-0000-0000-0000-000000000014"),
		CategoryID:   "travel",
		Amount:       "750.00",
		ApprovedAt:   time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
		Month:        "2026-04",
	}

	writer := &stubProjectionWriter{deletePendingErr: errors.New("foreign key violation")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectExpenseApproved,
		data:    mustMarshalEnvelope(t, events.SubjectExpenseApproved, tenantID, time.Now(), payload),
	}

	err := consumer.handleExpenseApproved(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on DeletePendingApproval must NAK for redelivery")
}

func TestConsumer_HandleProjectCreated_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000015")
	payload := events.ProjectCreatedPayload{
		ProjectID: uuid.MustParse("d2000000-0000-0000-0000-000000000015"),
		Title:     "Feature Film",
		Status:    "active",
	}

	writer := &stubProjectionWriter{upsertSnapshotErr: errors.New("deadlock detected")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectProjectCreated,
		data:    mustMarshalEnvelope(t, events.SubjectProjectCreated, tenantID, time.Now(), payload),
	}

	err := consumer.handleProjectCreated(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on UpsertPortfolioSnapshot must NAK for redelivery")
}

func TestConsumer_HandleProjectStatusChanged_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000016")
	payload := events.ProjectStatusChangedPayload{
		ProjectID:    uuid.MustParse("d2000000-0000-0000-0000-000000000016"),
		Title:        "Documentary",
		OldStatus:    "active",
		NewStatus:    "completed",
		CurrentPhase: "Wrap",
	}

	writer := &stubProjectionWriter{upsertSnapshotErr: errors.New("serialization failure")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectProjectStatusChanged,
		data:    mustMarshalEnvelope(t, events.SubjectProjectStatusChanged, tenantID, time.Now(), payload),
	}

	err := consumer.handleProjectStatusChanged(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on UpsertPortfolioSnapshot must NAK for redelivery")
}

func TestConsumer_HandleProjectMemberAssigned_DBError_NaksMessage(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000017")
	payload := events.ProjectMemberAssignedPayload{
		ProjectID:    uuid.MustParse("d2000000-0000-0000-0000-000000000017"),
		ProjectTitle: "TV Series",
		MemberCount:  25,
	}

	writer := &stubProjectionWriter{upsertTeamErr: errors.New("connection refused")}
	consumer := newTestConsumer(writer, newStubWatermarkStore())

	msg := &stubMessage{
		subject: events.SubjectProjectMemberAssigned,
		data:    mustMarshalEnvelope(t, events.SubjectProjectMemberAssigned, tenantID, time.Now(), payload),
	}

	err := consumer.handleProjectMemberAssigned(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "DB error on UpsertTeamAssignment must NAK for redelivery")
}
