# Billing migrations wiring + reconcile (#130) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make billing's schema managed and correct — wire `migrations/billing` into the tooling and rewrite `001` to match the live code — so `SuspendSubscription` and the #126 outbox work against a from-migrations DB.

**Architecture:** Three independent pieces: (1) rewrite `001` in place to match `services/billing/db/postgres.go` (safe — never persistently applied); (2) wire billing into `migrate-all`/`migrate-down`/`MIGRATION_DIRS`; (3) add billing's first real-DB integration test to prove the schema matches the code.

**Tech Stack:** golang-migrate v4 (per-dir `schema_migrations_<dir>`), Postgres, Go, `pkg/testdb`, testify.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-08-billing-migrations-130-design.md`.
- **No `services/billing` Go changes** — migrations + tooling + one new test file only.
- Edit `001` in place (it has never been applied to any persistent DB; golang-migrate v4 has no content checksums; CI runs it on fresh ephemeral DBs). Do NOT add a `002`.
- Billing must migrate AFTER `iam` (FKs `tenants`). It already sits after iam in CI's loops.
- Authoritative schema = what `services/billing/db/postgres.go` reads/writes; types from `services/billing/models.go`. Money columns `NUMERIC(14,2)`.
- Commits: Conventional Commits, scopes `billing` / `infra`. End every commit message with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

---

### Task 1: Rewrite `001` to match the live code

**Files:**
- Modify: `migrations/billing/001_create_tables.up.sql` (full rewrite)
- Modify: `migrations/billing/001_create_tables.down.sql` (full rewrite)

**Interfaces:**
- Produces: a `billing` schema (subscriptions/invoices/payment_methods/usage_records/invoice_sequences/dunning_attempts) that accepts every INSERT/UPDATE in `services/billing/db/postgres.go`.

- [ ] **Step 1: Rewrite the up migration**

Replace `migrations/billing/001_create_tables.up.sql` entirely with:

```sql
-- 001_create_tables.up.sql — billing service tables
-- Lives in the public (shared) schema — billing is a platform-level concern.
-- Schema matches services/billing/db/postgres.go (the live raw-SQL repository).

CREATE TABLE subscriptions (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID        NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    plan                 TEXT        NOT NULL DEFAULT 'starter'
                                     CHECK (plan IN ('starter', 'professional', 'enterprise')),
    status               TEXT        NOT NULL DEFAULT 'active'
                                     CHECK (status IN ('trialing', 'active', 'past_due', 'cancelled', 'suspended')),
    billing_cycle        TEXT        NOT NULL DEFAULT 'monthly'
                                     CHECK (billing_cycle IN ('monthly', 'annual')),
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end   TIMESTAMPTZ NOT NULL,
    trial_ends_at        TIMESTAMPTZ,
    cancelled_at         TIMESTAMPTZ,
    suspended_at         TIMESTAMPTZ,
    razorpay_sub_id      TEXT,
    stripe_sub_id        TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscriptions_status ON subscriptions (status);

CREATE TABLE invoices (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL REFERENCES tenants(id),
    subscription_id UUID          NOT NULL REFERENCES subscriptions(id),
    invoice_number  TEXT          NOT NULL UNIQUE,
    plan            TEXT          NOT NULL,
    amount          NUMERIC(14,2) NOT NULL,
    tax_amount      NUMERIC(14,2) NOT NULL DEFAULT 0,
    total_amount    NUMERIC(14,2) NOT NULL,
    currency        TEXT          NOT NULL DEFAULT 'INR',
    status          TEXT          NOT NULL DEFAULT 'pending'
                                  CHECK (status IN ('pending', 'paid', 'overdue', 'void', 'failed')),
    due_date        DATE          NOT NULL,
    period_start    TIMESTAMPTZ   NOT NULL,
    period_end      TIMESTAMPTZ   NOT NULL,
    paid_at         TIMESTAMPTZ,
    payment_method  TEXT,
    gateway_txn_id  TEXT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX idx_invoices_tenant ON invoices (tenant_id);
CREATE INDEX idx_invoices_status ON invoices (status) WHERE status IN ('pending', 'failed', 'overdue');

CREATE TABLE payment_methods (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type           TEXT        NOT NULL,
    display_name   TEXT        NOT NULL,
    is_default     BOOLEAN     NOT NULL DEFAULT false,
    razorpay_token TEXT,
    stripe_pm_id   TEXT,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payment_methods_tenant ON payment_methods (tenant_id);

CREATE TABLE usage_records (
    id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID          NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    period_start       TIMESTAMPTZ   NOT NULL,
    period_end         TIMESTAMPTZ   NOT NULL,
    active_productions INT           NOT NULL,
    user_count         INT           NOT NULL,
    storage_gb         NUMERIC(14,2) NOT NULL,
    recorded_at        TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX idx_usage_records_tenant ON usage_records (tenant_id);

CREATE TABLE invoice_sequences (
    year     INT PRIMARY KEY,
    last_seq INT NOT NULL DEFAULT 0
);

CREATE TABLE dunning_attempts (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id       UUID        NOT NULL REFERENCES invoices(id),
    attempt_number   INT         NOT NULL,
    outcome          TEXT        NOT NULL
                                 CHECK (outcome IN ('success', 'failed', 'card_declined', 'insufficient_funds')),
    gateway_response TEXT,
    next_retry_at    TIMESTAMPTZ,
    attempted_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dunning_invoice ON dunning_attempts (invoice_id);
```

Note on the `outcome` CHECK: the live repo writes `DunningAttempt.Result` verbatim into `outcome`; `models.go` documents the value domain as `success, failed, card_declined, insufficient_funds` (the only literal in the tree is `card_declined`; `queries.sql` filters `outcome='failed'`). The old `('succeeded','retrying')` values were never written by the code. Before committing, confirm no other literal is assigned to `DunningAttempt.Result` anywhere: `grep -rn "Result:" services/billing/ | grep -iv limitresult` — if a new value appears, widen the CHECK to include it.

- [ ] **Step 2: Rewrite the down migration**

Replace `migrations/billing/001_create_tables.down.sql` entirely with (reverse dependency order — children before parents):

```sql
DROP TABLE IF EXISTS dunning_attempts;
DROP TABLE IF EXISTS invoice_sequences;
DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS payment_methods;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS subscriptions;
```

- [ ] **Step 3: Verify up + down apply cleanly against a scratch DB**

`tenants` must exist first (billing FKs it). Bring up a scratch Postgres, apply iam then billing up, then billing down. Concretely (adapt container/DSN to the local setup; a disposable container is fine):

```bash
# against a scratch DB that already has iam's tenants table applied:
migrate -path migrations/iam     -database "$SCRATCH_DSN&x-migrations-table=schema_migrations_iam" up
migrate -path migrations/billing -database "$SCRATCH_DSN&x-migrations-table=schema_migrations_billing" up
migrate -path migrations/billing -database "$SCRATCH_DSN&x-migrations-table=schema_migrations_billing" down -all
```
Expected: billing up creates all six tables + indexes with no error (FKs to `tenants` resolve); billing down drops them cleanly. If no local Postgres is reachable, note it — CI's migration-validate job (`.github/workflows/ci.yml:185,197`, which already lists `billing`) exercises exactly this up+down on a fresh DB.

- [ ] **Step 4: Commit**

```bash
git add migrations/billing/001_create_tables.up.sql migrations/billing/001_create_tables.down.sql
git commit -m "fix(billing): reconcile migration 001 to the live schema (#130)

Add missing subscriptions/invoices columns, broaden status CHECKs, create
payment_methods/usage_records/invoice_sequences, add dunning next_retry_at.
Edit-in-place is safe: 001 was never applied to a persistent DB.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Wire billing into the migration tooling

**Files:**
- Modify: `Makefile` (`migrate-all` ~`:134-147`, `migrate-down` ~`:176-189`)
- Modify: `scripts/_db-common.sh` (`MIGRATION_DIRS` ~`:6-16`)

**Interfaces:**
- Consumes: Task 1's `migrations/billing` (must apply cleanly).
- Produces: `make migrate-all` (and dev/test bootstrap) creates billing's tables; drift-check covers billing.

- [ ] **Step 1: Add billing to `migrate-all`**

In `Makefile`'s `migrate-all` target, add this line immediately AFTER the `reporting` line and BEFORE the `audit` line (any post-`iam` position works; this matches CI's `…document billing audit`):

```make
	migrate -path migrations/billing       -database "$(DB_URL)&x-migrations-table=schema_migrations_billing" up
```

- [ ] **Step 2: Add billing to `migrate-down`**

In `Makefile`'s `migrate-down` target, add this line immediately AFTER the `audit` line (so billing rolls back before `iam`, which is last — billing FKs `tenants`):

```make
	migrate -path migrations/billing        -database "$(DB_URL)&x-migrations-table=schema_migrations_billing" down
```

(This line correctly carries `x-migrations-table`, matching the `audit`/`shared` lines. The other down lines omitting it is a pre-existing inconsistency — leave them untouched.)

- [ ] **Step 3: Add billing to `MIGRATION_DIRS`**

In `scripts/_db-common.sh`, add `billing` to the `MIGRATION_DIRS` array (after `notifications`, or any position — the array drives per-dir drift checks, order-independent):

```bash
MIGRATION_DIRS=(
  shared
  iam
  project
  budget
  expense
  ledger
  inventory
  document
  notifications
  billing
)
```

(`reporting`/`audit` are also absent from this array — pre-existing gaps, out of scope.)

- [ ] **Step 4: Verify migrate-all now provisions billing**

Grep confirms the wiring; a scratch run confirms end-to-end:
```bash
grep -n 'migrations/billing' Makefile          # expect two lines (up in migrate-all, down in migrate-down)
grep -n 'billing' scripts/_db-common.sh        # expect it in MIGRATION_DIRS
# end-to-end (if a scratch DB is reachable): a full make migrate-all creates billing tables after iam.
make migrate-all DB_URL="$SCRATCH_DSN"          # expect billing tables created, no error
```
Expected: both greps hit; `migrate-all` runs billing after iam with no FK error. If no local DB, the greps are the gate + CI's integration-tests job (`ci.yml:258`) applies the full chain.

- [ ] **Step 5: Commit**

```bash
git add Makefile scripts/_db-common.sh
git commit -m "infra(db): wire billing migrations into migrate-all/down + drift check (#130)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Billing's first real-DB integration test

**Files:**
- Create: `services/billing/db/subscription_integration_test.go`

**Interfaces:**
- Consumes: Task 1's schema + Task 2's wiring (so a bootstrapped test DB has billing's tables); the real `billingdb.NewPostgres` repo.
- Produces: a regression guard proving the migrated schema accepts the code's suspend write.

- [ ] **Step 1: Write the integration test**

Create `services/billing/db/subscription_integration_test.go`. It uses the pool directly (billing's `*Postgres` is pool-backed, so `testdb.NewTx` rollback-isolation doesn't apply) and cleans up via the `tenants` FK cascade in `t.Cleanup`:

```go
//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/billing"
	billingdb "github.com/wegofwd2020/thittam/services/billing/db"
)

// TestSubscriptionRoundTrip_SuspendFields proves the migrated billing schema
// (#130) accepts what the live repo writes — in particular the suspend fields
// (suspended_at, status='suspended') that migration 001 was missing before the
// reconcile. Guards against re-drift between code and migration.
func TestSubscriptionRoundTrip_SuspendFields(t *testing.T) {
	pool := testdb.Open(t) // owner role; connects to a migrated thittam_test
	ctx := context.Background()
	repo := billingdb.NewPostgres(pool)

	tenantID := uuid.New()
	// FK parent: subscriptions.tenant_id REFERENCES tenants(id). Seed one.
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status)
		 VALUES ($1, $2, $3, 'US', 'USD', 'active')`,
		tenantID, "Billing IT", "bil-"+tenantID.String()[:8])
	require.NoError(t, err, "seed tenant")
	t.Cleanup(func() {
		// ON DELETE CASCADE removes the subscription too.
		_, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	now := time.Now().UTC()
	sub := &billing.Subscription{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		Plan:               "starter",
		Status:             "active",
		BillingCycle:       "monthly",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	require.NoError(t, repo.CreateSubscription(ctx, sub), "create subscription")

	// The suspend write — exercises the columns 001 was missing.
	suspendedAt := now
	sub.Status = "suspended"
	sub.SuspendedAt = &suspendedAt
	sub.UpdatedAt = now
	require.NoError(t, repo.UpdateSubscription(ctx, sub), "suspend subscription")

	got, err := repo.GetSubscriptionByTenant(ctx, tenantID)
	require.NoError(t, err, "get subscription")
	assert.Equal(t, "suspended", got.Status)
	require.NotNil(t, got.SuspendedAt, "suspended_at must persist (the column 001 lacked)")
}
```

- [ ] **Step 2: Verify it compiles under the integration tag**

Run: `go test ./services/billing/db/ -tags=integration -run TestSubscriptionRoundTrip_SuspendFields -count=0`
Expected: compiles. (`-count=0` compiles without running.) If a bootstrapped test DB is available (`THITTAM_TEST_DSN` set + `make db-test-bootstrap` after Task 2), run it for real: `-count=1` — expect PASS. Without a DSN it SKIPs (via `testdb.Open`); CI's integration-tests job runs it against the migrated `thittam_test`.

- [ ] **Step 3: Confirm the whole tree still builds**

Run: `go build ./... && go vet ./...`
Expected: clean (new test file is behind the `integration` tag, so it doesn't affect the default build; `-tags=integration` build of the package must also succeed — Step 2 covers that).

- [ ] **Step 4: Commit**

```bash
git add services/billing/db/subscription_integration_test.go
git commit -m "test(billing): first real-DB integration test — subscription suspend round-trip (#130)

Proves the reconciled schema accepts the live repo's suspend write; guards
against code/migration re-drift.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Wire migrate-all/migrate-down/MIGRATION_DIRS → Task 2. ✓
- Rewrite 001 (subscriptions +5 cols + CHECK; invoices +4 cols + CHECK; payment_methods/usage_records/invoice_sequences; dunning +next_retry_at + verified CHECK; down drops new tables) → Task 1. ✓
- Edit-in-place (not additive 002) → Task 1, justified in Global Constraints. ✓
- First billing integration test exercising suspend fields → Task 3. ✓
- Non-goals (no service Go changes, no queries.sql/sqlc fix, no reporting/audit MIGRATION_DIRS) → honored; Task 3 is a new test file only, no service change. ✓

**Placeholder scan:** the dunning `outcome` CHECK is resolved to a concrete set with a one-line confirmation grep (not a deferred TODO). No other placeholders.

**Type consistency:** column types match `models.go` (`*time.Time`→nullable TIMESTAMPTZ; `decimal.Decimal`→NUMERIC(14,2); `string`→TEXT, nullable where the Go field is a plain string the code may leave empty — `razorpay_sub_id`/`stripe_sub_id`/`payment_method`/`gateway_txn_id` are nullable TEXT). The integration test's `billing.Subscription` fields match `models.go`. FK ordering (billing after iam) consistent across Task 1 (scratch verify), Task 2 (migrate-all position), Task 3 (seeds a tenant first).
