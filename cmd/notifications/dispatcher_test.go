package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/services/notifications"
)

// svcSender is a minimal ChannelSender that always succeeds.
type svcSender struct{}

func (s *svcSender) Send(_ context.Context, _ *notifications.DeliveryRequest) (string, error) {
	return "mock-msg-id", nil
}

// mockTemplateRepo returns a pre-configured template for any request.
// It implements the full notifications.Repository interface so tests can
// compile without depending on the real Postgres implementation.
type mockTemplateRepo struct {
	template *notifications.Template
	err      error
}

func (r *mockTemplateRepo) CreateTemplate(_ context.Context, _ *notifications.Template) error {
	return nil
}
func (r *mockTemplateRepo) UpdateTemplate(_ context.Context, _ *notifications.Template) error {
	return nil
}
func (r *mockTemplateRepo) GetTemplate(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*notifications.Template, error) {
	return nil, nil
}
func (r *mockTemplateRepo) GetActiveTemplate(_ context.Context, _ uuid.UUID, _ string, _ string) (*notifications.Template, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.template, nil
}
func (r *mockTemplateRepo) ListTemplates(_ context.Context, _ uuid.UUID) ([]notifications.Template, error) {
	return nil, nil
}
func (r *mockTemplateRepo) CreateNotification(_ context.Context, _ *notifications.Notification) error {
	return nil
}
func (r *mockTemplateRepo) UpdateNotificationStatus(_ context.Context, _ uuid.UUID, _, _, _ string, _ *time.Time) error {
	return nil
}
func (r *mockTemplateRepo) GetNotification(_ context.Context, _, _, _ uuid.UUID) (*notifications.Notification, error) {
	return nil, nil
}
func (r *mockTemplateRepo) ListNotifications(_ context.Context, _, _ uuid.UUID, _, _ string, _, _ int) ([]notifications.Notification, error) {
	return nil, nil
}
func (r *mockTemplateRepo) IncrementRetryCount(_ context.Context, _, _ uuid.UUID) error {
	return nil
}

// newTestDispatcher builds a dispatcher whose Send calls return
// ErrTemplateNotFound — exercising the ignoreTemplateMissing path.
func newTestDispatcher() *dispatcher {
	repo := &mockTemplateRepo{err: notifications.ErrTemplateNotFound}
	svc := notifications.NewService(repo, map[string]notifications.ChannelSender{"email": &svcSender{}})
	return &dispatcher{svc: svc}
}

// --- helpers ---

var testTenantID = uuid.MustParse("b0000000-0000-0000-0000-000000000001")

func makeEnvelope(t *testing.T, eventType string, payload any) *events.Envelope {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return &events.Envelope{
		EventID:    uuid.MustParse("e0000000-0000-0000-0000-000000000001"),
		Type:       eventType,
		TenantID:   testTenantID,
		OccurredAt: time.Now().UTC(),
		Payload:    data,
	}
}

// --- dispatch routing tests ---

func TestDispatch_UnknownEventType_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := &events.Envelope{
		EventID:  uuid.New(),
		Type:     "thittam.unknown.event",
		TenantID: testTenantID,
		Payload:  json.RawMessage(`{}`),
	}
	err := d.dispatch(context.Background(), env)
	assert.NoError(t, err, "unknown event type must be acked (forward-compatible)")
}

func TestDispatch_ExpenseSubmitted_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectExpenseSubmitted, events.ExpenseSubmittedPayload{
		ExpenseID:    uuid.MustParse("c0000000-0000-0000-0000-000000000001"),
		Amount:       "1500.00",
		SubmittedBy:  "Jane Crew",
		Description:  "Lighting hire",
		ProjectTitle: "Desert Shoot",
	})
	err := d.dispatch(context.Background(), env)
	assert.NoError(t, err, "missing template must be silently acked")
}

func TestDispatch_ExpenseApproved_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectExpenseApproved, events.ExpenseApprovedPayload{
		ExpenseID:    uuid.MustParse("c0000000-0000-0000-0000-000000000002"),
		Amount:       "750.00",
		CategoryName: "Transport",
		ApprovedAt:   time.Now().UTC(),
		Month:        "2026-04",
	})
	err := d.dispatch(context.Background(), env)
	assert.NoError(t, err)
}

func TestDispatch_BudgetApproved_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectBudgetApproved, events.BudgetApprovedPayload{
		BudgetID:       uuid.MustParse("c0000000-0000-0000-0000-000000000003"),
		ProductionID:   uuid.MustParse("c0000000-0000-0000-0000-000000000004"),
		TotalBudgeted:  "200000.00",
		TotalActual:    "180000.00",
		TotalCommitted: "185000.00",
	})
	err := d.dispatch(context.Background(), env)
	assert.NoError(t, err)
}

func TestDispatch_ProjectStatusChanged_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectProjectStatusChanged, events.ProjectStatusChangedPayload{
		ProjectID:    uuid.MustParse("c0000000-0000-0000-0000-000000000005"),
		Title:        "Desert Shoot",
		OldStatus:    "pre_production",
		NewStatus:    "production",
		CurrentPhase: "Day 1",
	})
	err := d.dispatch(context.Background(), env)
	assert.NoError(t, err)
}

func TestDispatch_DocumentSigned_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectDocumentSigned, events.DocumentSignedPayload{
		DocumentID:   "doc-001",
		DocumentName: "Crew Agreement",
		SignedBy:     uuid.MustParse("c0000000-0000-0000-0000-000000000006"),
		SignedAt:     time.Now().UTC(),
	})
	err := d.dispatch(context.Background(), env)
	assert.NoError(t, err)
}

func TestDispatch_ProjectCreated_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectProjectCreated, events.ProjectCreatedPayload{
		ProjectID: uuid.MustParse("c0000000-0000-0000-0000-000000000010"),
		Title:     "Desert Shoot",
		Status:    "pre_production",
	})
	assert.NoError(t, d.dispatch(context.Background(), env))
}

func TestDispatch_ProjectMemberAssigned_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectProjectMemberAssigned, events.ProjectMemberAssignedPayload{
		ProjectID:    uuid.MustParse("c0000000-0000-0000-0000-000000000011"),
		ProjectTitle: "Desert Shoot",
		MemberCount:  5,
	})
	assert.NoError(t, d.dispatch(context.Background(), env))
}

func TestDispatch_LedgerJournalPosted_TemplateMissing_ReturnsNil(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := makeEnvelope(t, events.SubjectLedgerJournalPosted, events.LedgerJournalPostedPayload{
		JournalEntryID: "je-001",
		EntryNumber:    "JE-2026-00001",
		TotalDebit:     "5000.00",
		TotalCredit:    "5000.00",
		Narration:      "Equipment hire",
		PostedAt:       time.Now().UTC(),
	})
	assert.NoError(t, d.dispatch(context.Background(), env))
}

func TestDispatch_MalformedPayload_ReturnsError(t *testing.T) {
	t.Parallel()
	d := newTestDispatcher()
	env := &events.Envelope{
		EventID:  uuid.New(),
		Type:     events.SubjectExpenseSubmitted,
		TenantID: testTenantID,
		Payload:  json.RawMessage(`not-valid-json`),
	}
	err := d.dispatch(context.Background(), env)
	assert.Error(t, err, "malformed payload must return error so NATS Naks and retries")
}

// --- ignoreTemplateMissing ---

func TestIgnoreTemplateMissing_ErrTemplateNotFound_ReturnsNil(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ignoreTemplateMissing(notifications.ErrTemplateNotFound))
}

func TestIgnoreTemplateMissing_OtherError_Propagates(t *testing.T) {
	t.Parallel()
	someErr := notifications.ErrInvalidChannel
	assert.ErrorIs(t, ignoreTemplateMissing(someErr), someErr)
}

func TestIgnoreTemplateMissing_Nil_ReturnsNil(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ignoreTemplateMissing(nil))
}
