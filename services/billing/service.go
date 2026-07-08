package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/events"
)

// Service implements billing business logic.
type Service struct {
	repo Repository
}

// NewService creates a billing service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// --- Subscription management ---

// CreateSubscription registers a new subscription for a tenant, starting a
// 14-day trial period. Returns ErrSubscriptionAlreadyExists if the tenant
// already has one.
func (s *Service) CreateSubscription(ctx context.Context, tenantID uuid.UUID, plan, billingCycle string) (*Subscription, error) {
	if _, ok := PlanLimitsByName[plan]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPlan, plan)
	}
	if billingCycle == "" {
		billingCycle = "monthly"
	}

	existing, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err == nil && existing != nil {
		return nil, ErrSubscriptionAlreadyExists
	}

	now := time.Now().UTC()
	trialEnd := now.Add(TrialDays * 24 * time.Hour)
	periodEnd := now.AddDate(0, 1, 0)
	if billingCycle == "annual" {
		periodEnd = now.AddDate(1, 0, 0)
	}

	sub := &Subscription{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		Plan:               plan,
		Status:             "trialing",
		BillingCycle:       billingCycle,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		TrialEndsAt:        &trialEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}
	return sub, nil
}

// GetSubscription retrieves the active subscription for a tenant.
func (s *Service) GetSubscription(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	sub, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return sub, nil
}

// UpgradeSubscription changes the plan on an existing subscription.
// Downgrades require the tenant to have freed resources to fit within the new
// plan limits — callers should validate with CheckPlanLimit before calling.
func (s *Service) UpgradeSubscription(ctx context.Context, tenantID uuid.UUID, newPlan string) (*Subscription, error) {
	if _, ok := PlanLimitsByName[newPlan]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPlan, newPlan)
	}

	sub, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	if sub.Status == "cancelled" {
		return nil, ErrSubscriptionCancelled
	}
	if sub.Status == "suspended" {
		return nil, ErrSubscriptionSuspended
	}

	sub.Plan = newPlan
	if sub.Status == "trialing" {
		sub.Status = "active"
	}
	sub.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateSubscription(ctx, sub); err != nil {
		return nil, fmt.Errorf("update subscription: %w", err)
	}
	return sub, nil
}

// CancelSubscription marks a subscription as cancelled. The subscription remains
// active until CurrentPeriodEnd.
func (s *Service) CancelSubscription(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	sub, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	if sub.Status == "cancelled" {
		return nil, ErrSubscriptionCancelled
	}

	now := time.Now().UTC()
	sub.Status = "cancelled"
	sub.CancelledAt = &now
	sub.UpdatedAt = now
	if err := s.repo.UpdateSubscription(ctx, sub); err != nil {
		return nil, fmt.Errorf("cancel subscription: %w", err)
	}
	return sub, nil
}

// SuspendSubscription moves a subscription to suspended and records the
// subscription.suspended event in the SAME transaction (transactional outbox,
// #126) — the relay publishes it. Atomic: a failed write fails the op.
func (s *Service) SuspendSubscription(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	sub, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	now := time.Now().UTC()
	sub.Status = "suspended"
	sub.SuspendedAt = &now
	sub.UpdatedAt = now

	payload, err := json.Marshal(events.BillingSubscriptionSuspendedPayload{
		SubscriptionID: sub.ID.String(),
		SuspendedAt:    now.Format(time.RFC3339),
		PurgeAfter:     now.AddDate(0, 0, 30).Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal suspend payload: %w", err)
	}

	if err := s.repo.SuspendSubscriptionWithOutbox(ctx, sub, events.SubjectBillingSubscriptionSuspended, payload); err != nil {
		return nil, fmt.Errorf("suspend subscription: %w", err)
	}
	return sub, nil
}

// --- Plan enforcement ---

// CheckPlanLimit verifies that the requested resource creation is within the
// tenant's plan limits. Returns ErrPlanLimitExceeded (wrapped) if not.
// Called by IAM and project-management service interceptors before creating
// productions or users.
func (s *Service) CheckPlanLimit(ctx context.Context, req CheckLimitRequest) (*LimitResult, error) {
	sub, err := s.repo.GetSubscriptionByTenant(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	if sub.Status == "suspended" {
		return &LimitResult{Allowed: false, Reason: "account is suspended"}, nil
	}

	limits, ok := PlanLimitsByName[sub.Plan]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPlan, sub.Plan)
	}

	switch req.Resource {
	case "production":
		if limits.MaxActiveProductions != -1 && req.CurrentProductions >= limits.MaxActiveProductions {
			return &LimitResult{
				Allowed: false,
				Reason: fmt.Sprintf("production limit reached (%d/%d) on %s plan",
					req.CurrentProductions, limits.MaxActiveProductions, sub.Plan),
			}, nil
		}
	case "user":
		if limits.MaxUsers != -1 && req.CurrentUsers >= limits.MaxUsers {
			return &LimitResult{
				Allowed: false,
				Reason: fmt.Sprintf("user limit reached (%d/%d) on %s plan",
					req.CurrentUsers, limits.MaxUsers, sub.Plan),
			}, nil
		}
	default:
		return nil, fmt.Errorf("billing: unknown resource type %q", req.Resource)
	}

	return &LimitResult{Allowed: true}, nil
}

// --- Invoices ---

// CreateInvoice generates an invoice with an auto-allocated sequential number
// (INV-YYYY-NNNNN). Tax is 18% GST for INR invoices; 0 for foreign currency.
func (s *Service) CreateInvoice(ctx context.Context, inv *Invoice) error {
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	if inv.InvoiceNumber == "" {
		year := time.Now().Year()
		seq, err := s.repo.NextInvoiceSeq(ctx, year)
		if err != nil {
			return fmt.Errorf("allocate invoice number: %w", err)
		}
		inv.InvoiceNumber = fmt.Sprintf("INV-%d-%05d", year, seq)
	}
	if inv.Status == "" {
		inv.Status = "pending"
	}
	// Apply 18% GST for INR invoices (SaaS SAC code 998314).
	if inv.Currency == "INR" && inv.TaxAmount.IsZero() {
		gst := decimal.NewFromFloat(0.18)
		inv.TaxAmount = inv.Amount.Mul(gst).Round(2)
	}
	inv.TotalAmount = inv.Amount.Add(inv.TaxAmount)
	inv.CreatedAt = time.Now().UTC()

	return s.repo.CreateInvoice(ctx, inv)
}

// GetInvoice retrieves an invoice by ID, scoped to the tenant.
func (s *Service) GetInvoice(ctx context.Context, tenantID, id uuid.UUID) (*Invoice, error) {
	return s.repo.GetInvoice(ctx, tenantID, id)
}

// ListInvoices lists invoices for a tenant, newest first.
func (s *Service) ListInvoices(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]Invoice, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListInvoices(ctx, tenantID, limit, offset)
}

// MarkInvoicePaid records a successful payment against an invoice.
func (s *Service) MarkInvoicePaid(ctx context.Context, tenantID, id uuid.UUID, gatewayTxnID string) (*Invoice, error) {
	inv, err := s.repo.GetInvoice(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	if inv.Status == "paid" {
		return nil, ErrInvoiceAlreadyPaid
	}

	now := time.Now().UTC()
	inv.Status = "paid"
	inv.PaidAt = &now
	inv.GatewayTxnID = gatewayTxnID
	if err := s.repo.UpdateInvoice(ctx, inv); err != nil {
		return nil, fmt.Errorf("update invoice: %w", err)
	}

	// Transition past_due subscription back to active on successful payment.
	sub, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err == nil && sub.Status == "past_due" {
		sub.Status = "active"
		sub.UpdatedAt = now
		_ = s.repo.UpdateSubscription(ctx, sub) // best-effort; non-critical
	}
	return inv, nil
}

// MarkInvoiceOverdue marks an invoice as overdue after all dunning retries fail.
func (s *Service) MarkInvoiceOverdue(ctx context.Context, tenantID, id uuid.UUID) (*Invoice, error) {
	inv, err := s.repo.GetInvoice(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}

	inv.Status = "overdue"
	if err := s.repo.UpdateInvoice(ctx, inv); err != nil {
		return nil, fmt.Errorf("update invoice: %w", err)
	}
	return inv, nil
}

// --- Payment methods ---

// AddPaymentMethod stores a tokenised payment method for a tenant.
// Raw card numbers must never be passed here — only gateway tokens.
func (s *Service) AddPaymentMethod(ctx context.Context, pm *PaymentMethod) error {
	if pm.ID == uuid.Nil {
		pm.ID = uuid.New()
	}
	pm.CreatedAt = time.Now().UTC()
	return s.repo.CreatePaymentMethod(ctx, pm)
}

// RemovePaymentMethod removes a stored payment method.
func (s *Service) RemovePaymentMethod(ctx context.Context, id uuid.UUID) error {
	_, err := s.repo.GetPaymentMethod(ctx, id)
	if err != nil {
		return fmt.Errorf("get payment method: %w", err)
	}
	return s.repo.DeletePaymentMethod(ctx, id)
}

// ListPaymentMethods returns payment methods for a tenant, capped at 20.
// A tenant realistically holds ≤5 saved cards; 20 is a generous safety ceiling.
func (s *Service) ListPaymentMethods(ctx context.Context, tenantID uuid.UUID) ([]PaymentMethod, error) {
	methods, err := s.repo.ListPaymentMethods(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const maxPaymentMethods = 20
	if len(methods) > maxPaymentMethods {
		methods = methods[:maxPaymentMethods]
	}
	return methods, nil
}

// SetDefaultPaymentMethod designates one payment method as the default for
// automatic charges. Clears the is_default flag from all others first.
func (s *Service) SetDefaultPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	pm, err := s.repo.GetPaymentMethod(ctx, id)
	if err != nil {
		return fmt.Errorf("get payment method: %w", err)
	}
	if err := s.repo.ClearDefaultPaymentMethods(ctx, tenantID); err != nil {
		return fmt.Errorf("clear defaults: %w", err)
	}
	pm.IsDefault = true
	return s.repo.UpdatePaymentMethod(ctx, pm)
}

// --- Usage metering ---

// RecordUsage persists a usage snapshot for a tenant. Called nightly.
func (s *Service) RecordUsage(ctx context.Context, u *UsageRecord) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	u.RecordedAt = time.Now().UTC()
	return s.repo.CreateUsageRecord(ctx, u)
}

// GetUsageSummary returns the latest usage record alongside the tenant's plan
// limits so the dashboard can render usage gauges.
func (s *Service) GetUsageSummary(ctx context.Context, tenantID uuid.UUID) (*UsageSummary, error) {
	sub, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	limits, ok := PlanLimitsByName[sub.Plan]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPlan, sub.Plan)
	}

	usage, err := s.repo.LatestUsageRecord(ctx, tenantID)
	if err != nil {
		// No usage record yet — return zeroed record.
		usage = &UsageRecord{TenantID: tenantID}
	}

	return &UsageSummary{
		TenantID:  tenantID,
		Plan:      sub.Plan,
		Usage:     *usage,
		Limits:    limits,
		UpdatedAt: usage.RecordedAt,
	}, nil
}

// --- Dunning ---

// RecordDunningAttempt appends a retry record for an overdue invoice.
func (s *Service) RecordDunningAttempt(ctx context.Context, d *DunningAttempt) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	d.AttemptedAt = time.Now().UTC()
	return s.repo.CreateDunningAttempt(ctx, d)
}

// ListDunningAttempts returns all retry records for an invoice.
func (s *Service) ListDunningAttempts(ctx context.Context, invoiceID uuid.UUID) ([]DunningAttempt, error) {
	return s.repo.ListDunningAttempts(ctx, invoiceID)
}
