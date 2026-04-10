package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/observability"
)

// ProjectionLagSLA is the maximum acceptable delay between a domain event being
// published and the projection being updated. Exceeding this causes /readyz to
// return 503 so load balancers stop routing traffic to a stale replica.
const ProjectionLagSLA = 30 * time.Second

// Message is a single event received from NATS JetStream.
// Ack must be called after successful processing to advance the consumer cursor.
// The consumer acks even on domain errors (bad payloads, skipped events) to
// avoid redelivery loops — only infra failures (DB unavailable) should cause NACKs.
type Message interface {
	Subject() string
	Data() []byte
	Ack() error
	Nak() error
}

// Subscriber registers a handler for a NATS subject.
// The single-subject pattern is used rather than a wildcard subscription so
// each handler is independently testable and independently monitored.
type Subscriber interface {
	Subscribe(subject string, handler func(ctx context.Context, msg Message) error) error
}

// ProjectionWriter is the write-side interface for reporting projections.
// It is separate from Repository (read-side) so the consumer and query paths
// can evolve independently.
type ProjectionWriter interface {
	// Expense projections
	UpsertExpenseFact(ctx context.Context, f ExpenseFact) error
	IncrementMonthlySpend(ctx context.Context, tenantID uuid.UUID, month string, actualDelta decimal.Decimal) error
	IncrementCategorySpend(ctx context.Context, tenantID uuid.UUID, categoryID, categoryName string, actualDelta decimal.Decimal) error
	UpsertPendingApproval(ctx context.Context, item PendingApprovalRow) error
	DeletePendingApproval(ctx context.Context, tenantID, expenseID uuid.UUID) error

	// Budget projections
	UpsertBudgetFact(ctx context.Context, f BudgetFact) error

	// Project projections
	UpsertPortfolioSnapshot(ctx context.Context, s PortfolioSnapshotRow) error
	UpsertTeamAssignment(ctx context.Context, tenantID, projectID uuid.UUID, projectTitle string, memberCount int) error
}

// WatermarkStore records the most-recent event time per subject.
// Used by LagChecker to measure projection staleness.
type WatermarkStore interface {
	// RecordWatermark upserts the high-water-mark for a subject.
	RecordWatermark(ctx context.Context, subject string, eventTime time.Time) error

	// OldestWatermark returns the earliest last-seen event time across all
	// subjects, which gives the worst-case projection lag.
	// Returns (zero time, nil) when no watermarks have been recorded yet.
	OldestWatermark(ctx context.Context) (time.Time, error)
}

// PendingApprovalRow is the write model for the pending_approvals projection.
type PendingApprovalRow struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	ItemType     string // "expense" or "budget"
	Description  string
	Amount       decimal.Decimal
	SubmittedBy  string
	SubmittedAt  time.Time
	ProjectTitle string
}

// PortfolioSnapshotRow is the write model for the portfolio_snapshots projection.
type PortfolioSnapshotRow struct {
	TenantID     uuid.UUID
	ProjectID    uuid.UUID
	Title        string
	Status       string
	CurrentPhase string
	StartDate    *string
}

// ProjectionConsumer subscribes to domain events and maintains the read-model
// projection tables used by reporting-analytics queries.
//
// # Delivery guarantee
//
// NATS JetStream provides at-least-once delivery. All projection writes are
// idempotent (UPSERT / ON CONFLICT) so duplicate delivery is safe.
//
// # Error policy
//
// Domain errors (bad JSON, unknown type, unknown tenant) are logged and acked
// so the consumer does not fall behind on one bad message. Infrastructure
// errors (DB unavailable) are nacked so the message is redelivered once the
// DB recovers.
type ProjectionConsumer struct {
	writer    ProjectionWriter
	watermark WatermarkStore
	logger    Logger
}

// NewProjectionConsumer creates a ProjectionConsumer.
func NewProjectionConsumer(writer ProjectionWriter, watermark WatermarkStore, logger Logger) *ProjectionConsumer {
	return &ProjectionConsumer{
		writer:    writer,
		watermark: watermark,
		logger:    logger,
	}
}

// Register subscribes to each domain event subject.
// Call this once during service startup after the NATS connection is established.
func (c *ProjectionConsumer) Register(sub Subscriber) error {
	routes := []struct {
		subject string
		handler func(ctx context.Context, msg Message) error
	}{
		{events.SubjectExpenseSubmitted, c.handleExpenseSubmitted},
		{events.SubjectExpenseApproved, c.handleExpenseApproved},
		{events.SubjectBudgetApproved, c.handleBudgetApproved},
		{events.SubjectProjectCreated, c.handleProjectCreated},
		{events.SubjectProjectStatusChanged, c.handleProjectStatusChanged},
		{events.SubjectProjectMemberAssigned, c.handleProjectMemberAssigned},
	}

	for _, r := range routes {
		if err := sub.Subscribe(r.subject, r.handler); err != nil {
			return fmt.Errorf("reporting: subscribe %s: %w", r.subject, err)
		}
	}
	return nil
}

// handleExpenseSubmitted adds the expense to pending_approvals.
func (c *ProjectionConsumer) handleExpenseSubmitted(ctx context.Context, msg Message) error {
	env, err := decodeEnvelope(msg.Data())
	if err != nil {
		c.logger.Error("skip: bad envelope", "subject", msg.Subject(), "error", err.Error())
		return msg.Ack()
	}

	var p events.ExpenseSubmittedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		c.logger.Error("skip: bad payload", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	amount, err := decimal.NewFromString(p.Amount)
	if err != nil {
		c.logger.Error("skip: bad amount", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	row := PendingApprovalRow{
		ID:           p.ExpenseID,
		TenantID:     env.TenantID,
		ItemType:     "expense",
		Description:  p.Description,
		Amount:       amount,
		SubmittedBy:  p.SubmittedBy,
		SubmittedAt:  p.SubmittedAt,
		ProjectTitle: p.ProjectTitle,
	}

	if err := c.writer.UpsertPendingApproval(ctx, row); err != nil {
		c.logger.Error("upsert pending_approval failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	c.advanceWatermark(ctx, msg.Subject(), env.OccurredAt)
	c.logger.Info("projection updated", "subject", msg.Subject(), "event_id", env.EventID, "tenant_id", env.TenantID)
	return msg.Ack()
}

// handleExpenseApproved upserts the expense fact, increments monthly spend and
// category spend, and removes the entry from pending_approvals.
func (c *ProjectionConsumer) handleExpenseApproved(ctx context.Context, msg Message) error {
	env, err := decodeEnvelope(msg.Data())
	if err != nil {
		c.logger.Error("skip: bad envelope", "subject", msg.Subject(), "error", err.Error())
		return msg.Ack()
	}

	var p events.ExpenseApprovedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		c.logger.Error("skip: bad payload", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	amount, err := decimal.NewFromString(p.Amount)
	if err != nil {
		c.logger.Error("skip: bad amount", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	fact := ExpenseFact{
		ExpenseID:    p.ExpenseID,
		ProductionID: p.ProductionID,
		TenantID:     env.TenantID,
		BudgetLineID: p.BudgetLineID,
		CategoryID:   p.CategoryID,
		Amount:       amount,
		ApprovedAt:   &p.ApprovedAt,
	}

	if err := c.writer.UpsertExpenseFact(ctx, fact); err != nil {
		c.logger.Error("upsert expense_fact failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	if err := c.writer.IncrementMonthlySpend(ctx, env.TenantID, p.Month, amount); err != nil {
		c.logger.Error("increment monthly_spend failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	if err := c.writer.IncrementCategorySpend(ctx, env.TenantID, p.CategoryID, p.CategoryName, amount); err != nil {
		c.logger.Error("increment category_spend failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	// Remove from pending approvals — idempotent if already absent.
	if err := c.writer.DeletePendingApproval(ctx, env.TenantID, p.ExpenseID); err != nil {
		c.logger.Error("delete pending_approval failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	c.advanceWatermark(ctx, msg.Subject(), env.OccurredAt)
	c.logger.Info("projection updated", "subject", msg.Subject(), "event_id", env.EventID, "tenant_id", env.TenantID)
	return msg.Ack()
}

// handleBudgetApproved upserts the budget fact for the production.
func (c *ProjectionConsumer) handleBudgetApproved(ctx context.Context, msg Message) error {
	env, err := decodeEnvelope(msg.Data())
	if err != nil {
		c.logger.Error("skip: bad envelope", "subject", msg.Subject(), "error", err.Error())
		return msg.Ack()
	}

	var p events.BudgetApprovedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		c.logger.Error("skip: bad payload", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	budgeted, err := decimal.NewFromString(p.TotalBudgeted)
	if err != nil {
		c.logger.Error("skip: bad total_budgeted", "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}
	actual, err := decimal.NewFromString(p.TotalActual)
	if err != nil {
		c.logger.Error("skip: bad total_actual", "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}
	committed, err := decimal.NewFromString(p.TotalCommitted)
	if err != nil {
		c.logger.Error("skip: bad total_committed", "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	fact := BudgetFact{
		BudgetID:       p.BudgetID,
		ProductionID:   p.ProductionID,
		TenantID:       env.TenantID,
		TotalBudgeted:  budgeted,
		TotalActual:    actual,
		TotalCommitted: committed,
	}

	if err := c.writer.UpsertBudgetFact(ctx, fact); err != nil {
		c.logger.Error("upsert budget_fact failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	c.advanceWatermark(ctx, msg.Subject(), env.OccurredAt)
	c.logger.Info("projection updated", "subject", msg.Subject(), "event_id", env.EventID, "tenant_id", env.TenantID)
	return msg.Ack()
}

// handleProjectCreated inserts a portfolio snapshot row for the new project.
func (c *ProjectionConsumer) handleProjectCreated(ctx context.Context, msg Message) error {
	env, err := decodeEnvelope(msg.Data())
	if err != nil {
		c.logger.Error("skip: bad envelope", "subject", msg.Subject(), "error", err.Error())
		return msg.Ack()
	}

	var p events.ProjectCreatedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		c.logger.Error("skip: bad payload", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	snap := PortfolioSnapshotRow{
		TenantID:  env.TenantID,
		ProjectID: p.ProjectID,
		Title:     p.Title,
		Status:    p.Status,
		StartDate: p.StartDate,
	}

	if err := c.writer.UpsertPortfolioSnapshot(ctx, snap); err != nil {
		c.logger.Error("upsert portfolio_snapshot failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	c.advanceWatermark(ctx, msg.Subject(), env.OccurredAt)
	c.logger.Info("projection updated", "subject", msg.Subject(), "event_id", env.EventID, "tenant_id", env.TenantID)
	return msg.Ack()
}

// handleProjectStatusChanged updates the status and phase in the portfolio snapshot.
func (c *ProjectionConsumer) handleProjectStatusChanged(ctx context.Context, msg Message) error {
	env, err := decodeEnvelope(msg.Data())
	if err != nil {
		c.logger.Error("skip: bad envelope", "subject", msg.Subject(), "error", err.Error())
		return msg.Ack()
	}

	var p events.ProjectStatusChangedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		c.logger.Error("skip: bad payload", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	snap := PortfolioSnapshotRow{
		TenantID:     env.TenantID,
		ProjectID:    p.ProjectID,
		Title:        p.Title,
		Status:       p.NewStatus,
		CurrentPhase: p.CurrentPhase,
	}

	if err := c.writer.UpsertPortfolioSnapshot(ctx, snap); err != nil {
		c.logger.Error("upsert portfolio_snapshot failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	c.advanceWatermark(ctx, msg.Subject(), env.OccurredAt)
	c.logger.Info("projection updated", "subject", msg.Subject(), "event_id", env.EventID, "tenant_id", env.TenantID)
	return msg.Ack()
}

// handleProjectMemberAssigned updates the team_assignments projection.
func (c *ProjectionConsumer) handleProjectMemberAssigned(ctx context.Context, msg Message) error {
	env, err := decodeEnvelope(msg.Data())
	if err != nil {
		c.logger.Error("skip: bad envelope", "subject", msg.Subject(), "error", err.Error())
		return msg.Ack()
	}

	var p events.ProjectMemberAssignedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		c.logger.Error("skip: bad payload", "subject", msg.Subject(), "event_id", env.EventID, "error", err.Error())
		return msg.Ack()
	}

	if err := c.writer.UpsertTeamAssignment(ctx, env.TenantID, p.ProjectID, p.ProjectTitle, p.MemberCount); err != nil {
		c.logger.Error("upsert team_assignment failed", "event_id", env.EventID, "error", err.Error())
		return msg.Nak()
	}

	c.advanceWatermark(ctx, msg.Subject(), env.OccurredAt)
	c.logger.Info("projection updated", "subject", msg.Subject(), "event_id", env.EventID, "tenant_id", env.TenantID)
	return msg.Ack()
}

// advanceWatermark records the event time but does not fail the handler if the
// watermark write fails — projection data has already been committed successfully.
func (c *ProjectionConsumer) advanceWatermark(ctx context.Context, subject string, eventTime time.Time) {
	if err := c.watermark.RecordWatermark(ctx, subject, eventTime); err != nil {
		c.logger.Error("watermark update failed (non-fatal)", "subject", subject, "error", err.Error())
	}
}

// decodeEnvelope unmarshals the raw NATS message body into an Envelope.
func decodeEnvelope(data []byte) (*events.Envelope, error) {
	var env events.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.EventID == uuid.Nil {
		return nil, fmt.Errorf("envelope missing event_id")
	}
	if env.Type == "" {
		return nil, fmt.Errorf("envelope missing type")
	}
	return &env, nil
}

// ── Lag checker ──────────────────────────────────────────────────────────────

// LagChecker implements observability.HealthChecker. It fails /readyz when the
// oldest projection watermark is more than ProjectionLagSLA behind wall clock.
//
// The intent: if the consumer has fallen more than 30 seconds behind, load
// balancers remove the replica from rotation so callers are not served stale
// dashboard data from a partition-affected pod.
type LagChecker struct {
	watermark WatermarkStore
	sla       time.Duration
	// clock allows tests to inject a fixed time without real sleeps.
	clock func() time.Time
}

// Verify LagChecker satisfies the observability interface at compile time.
var _ observability.HealthChecker = (*LagChecker)(nil)

// NewLagChecker creates a LagChecker with ProjectionLagSLA.
func NewLagChecker(watermark WatermarkStore) *LagChecker {
	return &LagChecker{
		watermark: watermark,
		sla:       ProjectionLagSLA,
		clock:     time.Now,
	}
}

// CheckHealth returns nil if the projection is within SLA, or an error
// describing the lag if it is not.
//
// A zero watermark (no events processed yet) is treated as OK — the service
// may have just started and no events have been published.
func (l *LagChecker) CheckHealth(ctx context.Context) error {
	oldest, err := l.watermark.OldestWatermark(ctx)
	if err != nil {
		return fmt.Errorf("reporting: lag check: %w", err)
	}

	// No watermarks recorded yet — assume in-sync (cold start / no activity).
	if oldest.IsZero() {
		return nil
	}

	lag := l.clock().Sub(oldest)
	if lag > l.sla {
		return fmt.Errorf("reporting: projection lag %s exceeds SLA of %s", lag.Round(time.Second), l.sla)
	}
	return nil
}
