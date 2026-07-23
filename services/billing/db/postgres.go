package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/billing"
)

// Postgres implements billing.Repository backed by PostgreSQL.
// The sqlc-generated models in this package cover the IAM/billing shared schema
// but lack several billing-specific columns (plan, total_amount, gateway_txn_id,
// trial_ends_at, etc.). All billing operations use raw parameterised SQL to
// maintain full field coverage without regenerating sqlc.
type Postgres struct {
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed billing repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{db: db}
}

// Compile-time interface check.
var _ billing.Repository = (*Postgres)(nil)

// --- Subscriptions ---

func (p *Postgres) CreateSubscription(ctx context.Context, s *billing.Subscription) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO subscriptions
		  (id, tenant_id, plan, status, billing_cycle,
		   current_period_start, current_period_end,
		   trial_ends_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (tenant_id) DO NOTHING`,
		s.ID, s.TenantID, s.Plan, s.Status, s.BillingCycle,
		s.CurrentPeriodStart, s.CurrentPeriodEnd,
		s.TrialEndsAt, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("billing: create subscription: %w", err)
	}
	return nil
}

func (p *Postgres) GetSubscriptionByTenant(ctx context.Context, tenantID uuid.UUID) (*billing.Subscription, error) {
	row := p.db.QueryRow(ctx, `
		SELECT id, tenant_id, plan, status, billing_cycle,
		       current_period_start, current_period_end,
		       trial_ends_at, cancelled_at, suspended_at,
		       razorpay_sub_id, stripe_sub_id,
		       created_at, updated_at
		FROM subscriptions WHERE tenant_id = $1`, tenantID)

	s := &billing.Subscription{}
	var trialEndsAt, cancelledAt, suspendedAt pgtype.Timestamptz
	var razorpaySubID, stripeSubID pgtype.Text
	err := row.Scan(
		&s.ID, &s.TenantID, &s.Plan, &s.Status, &s.BillingCycle,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&trialEndsAt, &cancelledAt, &suspendedAt,
		&razorpaySubID, &stripeSubID,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, billing.ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("billing: get subscription by tenant: %w", err)
	}
	if trialEndsAt.Valid {
		s.TrialEndsAt = &trialEndsAt.Time
	}
	if cancelledAt.Valid {
		s.CancelledAt = &cancelledAt.Time
	}
	if suspendedAt.Valid {
		s.SuspendedAt = &suspendedAt.Time
	}
	if razorpaySubID.Valid {
		s.RazorpaySubID = razorpaySubID.String
	}
	if stripeSubID.Valid {
		s.StripeSubID = stripeSubID.String
	}
	return s, nil
}

func (p *Postgres) UpdateSubscription(ctx context.Context, s *billing.Subscription) error {
	_, err := p.db.Exec(ctx, `
		UPDATE subscriptions
		SET plan                  = $2,
		    status                = $3,
		    billing_cycle         = $4,
		    current_period_start  = $5,
		    current_period_end    = $6,
		    trial_ends_at         = $7,
		    cancelled_at          = $8,
		    suspended_at          = $9,
		    razorpay_sub_id       = $10,
		    stripe_sub_id         = $11,
		    updated_at            = $12
		WHERE tenant_id = $1`,
		s.TenantID, s.Plan, s.Status, s.BillingCycle,
		s.CurrentPeriodStart, s.CurrentPeriodEnd,
		s.TrialEndsAt, s.CancelledAt, s.SuspendedAt,
		s.RazorpaySubID, s.StripeSubID,
		s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("billing: update subscription: %w", err)
	}
	return nil
}

// --- Outbox (#126) ---

// SuspendSubscriptionWithOutbox atomically suspends a subscription and
// records a subscription.suspended outbox row in one tx so the domain
// change and the event are never observed inconsistently.
func (p *Postgres) SuspendSubscriptionWithOutbox(ctx context.Context, s *billing.Subscription, subject string, payload []byte) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: begin suspend+outbox tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Same column set as UpdateSubscription — keep in sync.
	if _, err := tx.Exec(ctx, `
		UPDATE subscriptions
		SET plan=$2, status=$3, billing_cycle=$4, current_period_start=$5, current_period_end=$6,
		    trial_ends_at=$7, cancelled_at=$8, suspended_at=$9, razorpay_sub_id=$10, stripe_sub_id=$11,
		    updated_at=$12
		WHERE tenant_id = $1`,
		s.TenantID, s.Plan, s.Status, s.BillingCycle, s.CurrentPeriodStart, s.CurrentPeriodEnd,
		s.TrialEndsAt, s.CancelledAt, s.SuspendedAt, s.RazorpaySubID, s.StripeSubID, s.UpdatedAt,
	); err != nil {
		return fmt.Errorf("billing: suspend subscription (tx): %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO event_outbox (subject, tenant_id, payload) VALUES ($1, $2, $3)`,
		subject, s.TenantID, payload,
	); err != nil {
		return fmt.Errorf("billing: insert outbox: %w", err)
	}

	return tx.Commit(ctx)
}

// ClaimUnsentOutbox atomically claims a batch of unsent events for the relay:
// SKIP LOCKED avoids double-claim across replicas; attempts++ counts the try.
// The claim is a single short statement — no tx is held across the NATS publish.
func (p *Postgres) ClaimUnsentOutbox(ctx context.Context, limit int) ([]*billing.OutboxEvent, error) {
	rows, err := p.db.Query(ctx, `
		UPDATE event_outbox SET attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM event_outbox
			WHERE sent_at IS NULL
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		RETURNING id, subject, tenant_id, payload, created_at, sent_at, attempts, last_error`, limit)
	if err != nil {
		return nil, fmt.Errorf("billing: claim outbox: %w", err)
	}
	defer rows.Close()

	var out []*billing.OutboxEvent
	for rows.Next() {
		var e billing.OutboxEvent
		var sentAt pgtype.Timestamptz
		var lastErr pgtype.Text
		if err := rows.Scan(&e.ID, &e.Subject, &e.TenantID, &e.Payload, &e.CreatedAt, &sentAt, &e.Attempts, &lastErr); err != nil {
			return nil, fmt.Errorf("billing: scan outbox: %w", err)
		}
		if sentAt.Valid {
			e.SentAt = &sentAt.Time
		}
		if lastErr.Valid {
			e.LastError = &lastErr.String
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (p *Postgres) MarkOutboxSent(ctx context.Context, id uuid.UUID) error {
	if _, err := p.db.Exec(ctx, `UPDATE event_outbox SET sent_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("billing: mark outbox sent: %w", err)
	}
	return nil
}

func (p *Postgres) RecordOutboxFailure(ctx context.Context, id uuid.UUID, errMsg string) error {
	if _, err := p.db.Exec(ctx, `UPDATE event_outbox SET last_error = $2 WHERE id = $1`, id, errMsg); err != nil {
		return fmt.Errorf("billing: record outbox failure: %w", err)
	}
	return nil
}

func (p *Postgres) DeleteSentOutboxOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	ct, err := p.db.Exec(ctx, `DELETE FROM event_outbox WHERE sent_at IS NOT NULL AND sent_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("billing: delete sent outbox: %w", err)
	}
	return ct.RowsAffected(), nil
}

// MoveOutboxToDead parks a poison event: copy it into event_outbox_dead and
// delete it from event_outbox, in one tx. Idempotent — if a concurrent relay
// already moved the row, zero rows are copied and the tx commits as a no-op.
func (p *Postgres) MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: begin move-to-dead tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO event_outbox_dead (id, subject, tenant_id, payload, created_at, attempts, last_error, died_at)
		SELECT id, subject, tenant_id, payload, created_at, attempts, $2, now()
		FROM event_outbox WHERE id = $1
		ON CONFLICT (id) DO NOTHING`, id, errMsg); err != nil {
		return fmt.Errorf("billing: insert dead outbox: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM event_outbox WHERE id = $1`, id); err != nil {
		return fmt.Errorf("billing: delete moved outbox: %w", err)
	}

	return tx.Commit(ctx)
}

// OutboxStats reads pending depth, oldest-pending age, and dead depth in one
// round trip. The first two subqueries ride idx_event_outbox_unsent.
func (p *Postgres) OutboxStats(ctx context.Context) (*billing.OutboxStats, error) {
	var s billing.OutboxStats
	err := p.db.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM event_outbox WHERE sent_at IS NULL),
		  (SELECT coalesce(extract(epoch FROM now() - min(created_at)), 0)::double precision
		     FROM event_outbox WHERE sent_at IS NULL),
		  (SELECT count(*) FROM event_outbox_dead)`).
		Scan(&s.Pending, &s.OldestPendingSeconds, &s.Dead)
	if err != nil {
		return nil, fmt.Errorf("billing: outbox stats: %w", err)
	}
	return &s, nil
}

// ListDeadOutbox returns parked events, most recently died first.
func (p *Postgres) ListDeadOutbox(ctx context.Context, limit int) ([]*billing.OutboxEvent, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, subject, tenant_id, payload, created_at, attempts, last_error
		FROM event_outbox_dead
		ORDER BY died_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("billing: list dead outbox: %w", err)
	}
	defer rows.Close()

	var out []*billing.OutboxEvent
	for rows.Next() {
		var e billing.OutboxEvent
		var lastErr pgtype.Text
		if err := rows.Scan(&e.ID, &e.Subject, &e.TenantID, &e.Payload, &e.CreatedAt, &e.Attempts, &lastErr); err != nil {
			return nil, fmt.Errorf("billing: scan dead outbox: %w", err)
		}
		if lastErr.Valid {
			e.LastError = &lastErr.String
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ReplayDeadOutbox re-arms a parked event: move it back to event_outbox with
// attempts reset and last_error cleared, preserving id and created_at so
// age-based alerts stay honest. Unknown id → ErrOutboxEventNotFound.
func (p *Postgres) ReplayDeadOutbox(ctx context.Context, id uuid.UUID) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: begin replay tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ct, err := tx.Exec(ctx, `
		INSERT INTO event_outbox (id, subject, tenant_id, payload, created_at, sent_at, attempts, last_error)
		SELECT id, subject, tenant_id, payload, created_at, NULL, 0, NULL
		FROM event_outbox_dead WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("billing: reinsert outbox: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return billing.ErrOutboxEventNotFound
	}

	if _, err := tx.Exec(ctx, `DELETE FROM event_outbox_dead WHERE id = $1`, id); err != nil {
		return fmt.Errorf("billing: delete replayed outbox: %w", err)
	}

	return tx.Commit(ctx)
}

// --- Invoices ---

func (p *Postgres) CreateInvoice(ctx context.Context, inv *billing.Invoice) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO invoices
		  (id, tenant_id, subscription_id, invoice_number,
		   plan, amount, tax_amount, total_amount, currency,
		   status, due_date, period_start, period_end, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO NOTHING`,
		inv.ID, inv.TenantID, inv.SubscriptionID, inv.InvoiceNumber,
		inv.Plan,
		pgNumericFromDecimal(inv.Amount),
		pgNumericFromDecimal(inv.TaxAmount),
		pgNumericFromDecimal(inv.TotalAmount),
		inv.Currency,
		inv.Status,
		inv.DueDate,
		inv.PeriodStart, inv.PeriodEnd,
		inv.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("billing: create invoice: %w", err)
	}
	return nil
}

func (p *Postgres) GetInvoice(ctx context.Context, tenantID, id uuid.UUID) (*billing.Invoice, error) {
	row := p.db.QueryRow(ctx, `
		SELECT id, tenant_id, subscription_id, invoice_number,
		       plan, amount, tax_amount, total_amount, currency,
		       status, due_date, period_start, period_end,
		       paid_at, payment_method, gateway_txn_id, created_at
		FROM invoices WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return scanInvoice(row)
}

func (p *Postgres) ListInvoices(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]billing.Invoice, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, tenant_id, subscription_id, invoice_number,
		       plan, amount, tax_amount, total_amount, currency,
		       status, due_date, period_start, period_end,
		       paid_at, payment_method, gateway_txn_id, created_at
		FROM invoices WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("billing: list invoices: %w", err)
	}
	defer rows.Close()

	var result []billing.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *inv)
	}
	return result, rows.Err()
}

func (p *Postgres) UpdateInvoice(ctx context.Context, inv *billing.Invoice) error {
	_, err := p.db.Exec(ctx, `
		UPDATE invoices
		SET status          = $3,
		    paid_at         = $4,
		    gateway_txn_id  = $5,
		    payment_method  = $6
		WHERE id = $1 AND tenant_id = $2`,
		inv.ID, inv.TenantID,
		inv.Status,
		inv.PaidAt,
		inv.GatewayTxnID,
		inv.PaymentMethod,
	)
	if err != nil {
		return fmt.Errorf("billing: update invoice: %w", err)
	}
	return nil
}

// NextInvoiceSeq returns the next sequential invoice number for the given year
// using a PostgreSQL advisory lock + counter row (idempotent, no gaps under
// normal operation but not guaranteed gap-free across rollbacks).
func (p *Postgres) NextInvoiceSeq(ctx context.Context, year int) (int, error) {
	var seq int
	err := p.db.QueryRow(ctx, `
		INSERT INTO invoice_sequences (year, last_seq)
		VALUES ($1, 1)
		ON CONFLICT (year) DO UPDATE
		  SET last_seq = invoice_sequences.last_seq + 1
		RETURNING last_seq`, year,
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("billing: next invoice seq: %w", err)
	}
	return seq, nil
}

// --- Payment Methods ---

func (p *Postgres) CreatePaymentMethod(ctx context.Context, pm *billing.PaymentMethod) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO payment_methods
		  (id, tenant_id, type, display_name, is_default,
		   razorpay_token, stripe_pm_id, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO NOTHING`,
		pm.ID, pm.TenantID, pm.Type, pm.DisplayName, pm.IsDefault,
		pm.RazorpayToken, pm.StripePMID,
		pm.ExpiresAt, pm.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("billing: create payment method: %w", err)
	}
	return nil
}

func (p *Postgres) GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*billing.PaymentMethod, error) {
	row := p.db.QueryRow(ctx, `
		SELECT id, tenant_id, type, display_name, is_default,
		       razorpay_token, stripe_pm_id, expires_at, created_at
		FROM payment_methods WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return scanPaymentMethod(row)
}

func (p *Postgres) ListPaymentMethods(ctx context.Context, tenantID uuid.UUID) ([]billing.PaymentMethod, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, tenant_id, type, display_name, is_default,
		       razorpay_token, stripe_pm_id, expires_at, created_at
		FROM payment_methods WHERE tenant_id = $1
		ORDER BY is_default DESC, created_at ASC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("billing: list payment methods: %w", err)
	}
	defer rows.Close()

	var result []billing.PaymentMethod
	for rows.Next() {
		pm, err := scanPaymentMethod(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *pm)
	}
	return result, rows.Err()
}

func (p *Postgres) UpdatePaymentMethod(ctx context.Context, pm *billing.PaymentMethod) error {
	tag, err := p.db.Exec(ctx, `
		UPDATE payment_methods
		SET is_default = $2, expires_at = $3
		WHERE id = $1 AND tenant_id = $4`, pm.ID, pm.IsDefault, pm.ExpiresAt, pm.TenantID)
	if err != nil {
		return fmt.Errorf("billing: update payment method: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return billing.ErrPaymentMethodNotFound
	}
	return nil
}

func (p *Postgres) DeletePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM payment_methods WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("billing: delete payment method: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return billing.ErrPaymentMethodNotFound
	}
	return nil
}

func (p *Postgres) ClearDefaultPaymentMethods(ctx context.Context, tenantID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `
		UPDATE payment_methods SET is_default = false WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return fmt.Errorf("billing: clear default payment methods: %w", err)
	}
	return nil
}

// --- Usage records ---

func (p *Postgres) CreateUsageRecord(ctx context.Context, u *billing.UsageRecord) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO usage_records
		  (id, tenant_id, period_start, period_end,
		   active_productions, user_count, storage_gb, recorded_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO NOTHING`,
		u.ID, u.TenantID, u.PeriodStart, u.PeriodEnd,
		u.ActiveProductions, u.UserCount,
		pgNumericFromDecimal(u.StorageGB),
		u.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf("billing: create usage record: %w", err)
	}
	return nil
}

func (p *Postgres) LatestUsageRecord(ctx context.Context, tenantID uuid.UUID) (*billing.UsageRecord, error) {
	row := p.db.QueryRow(ctx, `
		SELECT id, tenant_id, period_start, period_end,
		       active_productions, user_count, storage_gb, recorded_at
		FROM usage_records WHERE tenant_id = $1
		ORDER BY recorded_at DESC
		LIMIT 1`, tenantID)

	u := &billing.UsageRecord{}
	var storageGB pgtype.Numeric
	err := row.Scan(
		&u.ID, &u.TenantID, &u.PeriodStart, &u.PeriodEnd,
		&u.ActiveProductions, &u.UserCount, &storageGB, &u.RecordedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("billing: no usage record found")
		}
		return nil, fmt.Errorf("billing: latest usage record: %w", err)
	}
	u.StorageGB = decimalFromPgNumeric(storageGB)
	return u, nil
}

// --- Dunning ---

func (p *Postgres) CreateDunningAttempt(ctx context.Context, d *billing.DunningAttempt) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO dunning_attempts
		  (id, invoice_id, attempt_number, outcome, attempted_at, next_retry_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO NOTHING`,
		d.ID, d.InvoiceID, d.AttemptNumber, d.Result,
		d.AttemptedAt, d.NextRetryAt,
	)
	if err != nil {
		return fmt.Errorf("billing: create dunning attempt: %w", err)
	}
	return nil
}

func (p *Postgres) ListDunningAttempts(ctx context.Context, invoiceID uuid.UUID) ([]billing.DunningAttempt, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, invoice_id, attempt_number, outcome, attempted_at, next_retry_at
		FROM dunning_attempts WHERE invoice_id = $1
		ORDER BY attempt_number ASC`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("billing: list dunning attempts: %w", err)
	}
	defer rows.Close()

	var result []billing.DunningAttempt
	for rows.Next() {
		var d billing.DunningAttempt
		var nextRetryAt pgtype.Timestamptz
		if err := rows.Scan(
			&d.ID, &d.InvoiceID, &d.AttemptNumber, &d.Result,
			&d.AttemptedAt, &nextRetryAt,
		); err != nil {
			return nil, fmt.Errorf("billing: scan dunning attempt: %w", err)
		}
		if nextRetryAt.Valid {
			d.NextRetryAt = &nextRetryAt.Time
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// --- row scanners ---

// pgxRow is satisfied by both *pgx.Row and pgx.Rows.
type pgxRow interface {
	Scan(dest ...any) error
}

func scanInvoice(row pgxRow) (*billing.Invoice, error) {
	inv := &billing.Invoice{}
	var amount, taxAmount, totalAmount pgtype.Numeric
	var paidAt pgtype.Timestamptz
	var paymentMethod, gatewayTxnID pgtype.Text
	err := row.Scan(
		&inv.ID, &inv.TenantID, &inv.SubscriptionID, &inv.InvoiceNumber,
		&inv.Plan,
		&amount, &taxAmount, &totalAmount,
		&inv.Currency, &inv.Status,
		&inv.DueDate, &inv.PeriodStart, &inv.PeriodEnd,
		&paidAt, &paymentMethod, &gatewayTxnID,
		&inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, billing.ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("billing: scan invoice: %w", err)
	}
	inv.Amount = decimalFromPgNumeric(amount)
	inv.TaxAmount = decimalFromPgNumeric(taxAmount)
	inv.TotalAmount = decimalFromPgNumeric(totalAmount)
	if paidAt.Valid {
		inv.PaidAt = &paidAt.Time
	}
	if paymentMethod.Valid {
		inv.PaymentMethod = paymentMethod.String
	}
	if gatewayTxnID.Valid {
		inv.GatewayTxnID = gatewayTxnID.String
	}
	return inv, nil
}

func scanPaymentMethod(row pgxRow) (*billing.PaymentMethod, error) {
	pm := &billing.PaymentMethod{}
	var razorpayToken, stripePMID pgtype.Text
	var expiresAt pgtype.Timestamptz
	err := row.Scan(
		&pm.ID, &pm.TenantID, &pm.Type, &pm.DisplayName, &pm.IsDefault,
		&razorpayToken, &stripePMID,
		&expiresAt, &pm.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, billing.ErrPaymentMethodNotFound
		}
		return nil, fmt.Errorf("billing: scan payment method: %w", err)
	}
	if razorpayToken.Valid {
		pm.RazorpayToken = razorpayToken.String
	}
	if stripePMID.Valid {
		pm.StripePMID = stripePMID.String
	}
	if expiresAt.Valid {
		pm.ExpiresAt = &expiresAt.Time
	}
	return pm, nil
}

// --- pgtype conversion helpers ---

func pgNumericFromDecimal(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func decimalFromPgNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

