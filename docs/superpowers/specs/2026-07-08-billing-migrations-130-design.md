# Design: wire billing migrations + reconcile 001 to live code (#130)

**Issue:** #130 (pre-existing billing-schema drift, surfaced while scoping #126). **Date:** 2026-07-08
**Scope:** migrations/billing + migration tooling + a first billing integration test. No service-code changes.
**Branch:** `feat/billing-migrations-130` (to be created)
**Blocks:** #126 (billing outbox) — resumes on this clean base once merged.

## Context

Two coupled pre-existing defects leave billing's schema unmanaged:

1. **`migrations/billing` is wired into nothing** — absent from the `migrate-all` / `migrate-down`
   Makefile targets and from `MIGRATION_DIRS` in `scripts/_db-common.sh`. `make migrate-all`
   (used by `db-bootstrap.sh:34` and `db-test-bootstrap.sh:19`) never creates billing's tables.
2. **`001_create_tables.up.sql` has drifted behind the live code** (`services/billing/db/postgres.go`
   uses raw SQL over tables/columns `001` doesn't define). `SuspendSubscription` writes
   `suspended_at` — a column `001` lacks — so it would fail against a from-migrations schema.

**Why editing `001` in place is safe** (not an additive `002`): `001` has never been applied to any
persistent DB. The ONLY consumer is CI (`.github/workflows/ci.yml:185,197,258`), which applies it to
ephemeral, freshly-created Postgres containers (up, and down in migration-validate) and tears them
down each run. No `schema_migrations_billing` row exists in any dev/test DB. golang-migrate v4.17.1
tracks only `(version, dirty)` — **no content checksums** — so editing `001` re-runs cleanly wherever
version 1 was never recorded (i.e. everywhere persistent). Edit-in-place yields one correct migration
instead of a `001` + reconciling-`002` with `IF NOT EXISTS` scaffolding.

**Why the drift never turned CI red:** migration-validate runs the SQL but no Go code; billing has
**zero** integration tests touching a real DB (only mock-based unit tests). So missing
tables/columns/over-narrow CHECKs were never exercised. Component 3 closes that gap.

## Component 1 — wire billing into the migration tooling

- **`Makefile` `migrate-all`** (`:134-147`): add, after the `reporting` line and before `audit`
  (billing FKs `tenants` from `iam`, which runs first; any post-iam slot is valid — this matches CI's
  `…document billing audit` ordering):
  ```make
  migrate -path migrations/billing       -database "$(DB_URL)&x-migrations-table=schema_migrations_billing" up
  ```
- **`Makefile` `migrate-down`** (`:176-189`): add the reverse-order line with the same
  `x-migrations-table=schema_migrations_billing` (matching the `audit`/`shared` lines; the other
  down lines' missing `x-migrations-table` is a pre-existing inconsistency left untouched).
- **`scripts/_db-common.sh` `MIGRATION_DIRS`** (`:6-16`): add `billing` so `check_migration_head`
  covers it. (`reporting`/`audit` are also absent from this array — pre-existing, out of scope.)

## Component 2 — rewrite `001` to match the live code

Authoritative source = what `services/billing/db/postgres.go` reads/writes; types from
`services/billing/models.go`. Full delta:

**`subscriptions`** — add nullable columns + broaden the status CHECK:
```sql
    trial_ends_at   TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ,
    suspended_at    TIMESTAMPTZ,
    razorpay_sub_id TEXT,
    stripe_sub_id   TEXT,
    -- status CHECK becomes:
    CHECK (status IN ('trialing','active','past_due','cancelled','suspended'))
```

**`invoices`** — add columns + broaden the status CHECK:
```sql
    plan           TEXT          NOT NULL,
    total_amount   NUMERIC(14,2) NOT NULL,
    payment_method TEXT,
    gateway_txn_id TEXT,
    -- status CHECK becomes:
    CHECK (status IN ('pending','paid','overdue','void','failed'))
```
(`invoice_number`/`amount`/`tax_amount`/`currency`/`due_date`/`period_start`/`period_end`/`paid_at`
stay. `plan`/`total_amount` are NOT NULL because the code always inserts them.)

**New table `payment_methods`** (`postgres.go:224-297`, `models.go:53-63`):
```sql
CREATE TABLE payment_methods (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type           TEXT        NOT NULL,          -- card, upi, netbanking, bank_transfer
    display_name   TEXT        NOT NULL,
    is_default     BOOLEAN     NOT NULL DEFAULT false,
    razorpay_token TEXT,
    stripe_pm_id   TEXT,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payment_methods_tenant ON payment_methods (tenant_id);
```

**New table `usage_records`** (`postgres.go:301-341`, `models.go:67-76`):
```sql
CREATE TABLE usage_records (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    period_start       TIMESTAMPTZ NOT NULL,
    period_end         TIMESTAMPTZ NOT NULL,
    active_productions INT         NOT NULL,
    user_count         INT         NOT NULL,
    storage_gb         NUMERIC(14,2) NOT NULL,
    recorded_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_usage_records_tenant ON usage_records (tenant_id);
```

**New table `invoice_sequences`** (`postgres.go:207-220`, upsert `ON CONFLICT (year)`):
```sql
CREATE TABLE invoice_sequences (
    year     INT PRIMARY KEY,
    last_seq INT NOT NULL DEFAULT 0
);
```

**`dunning_attempts`** — add `next_retry_at TIMESTAMPTZ` (`postgres.go:348`). **The `outcome` CHECK
must be verified against the actual write path before finalizing** — `001` allows
`('succeeded','failed','retrying')` but `models.go:84` documents `('success','failed',
'card_declined','insufficient_funds')`, and `postgres.go` writes `d.Result` straight into `outcome`.
Implementation MUST read the dunning write path (service + repo) and set the CHECK to the exact set
of literals written (or drop the CHECK if the set is open-ended) — do NOT guess. `gateway_response`
(present in `001`, used by the sqlc query but not by `postgres.go`) stays.

**`001…down.sql`**: extend to drop the three new tables in reverse-dependency order (before/around
the existing drops): `usage_records`, `payment_methods`, `invoice_sequences`, then
`dunning_attempts`, `invoices`, `subscriptions`.

## Component 3 — billing's first integration test

New `services/billing/db/subscription_integration_test.go` (`//go:build integration`, `pkg/testdb`,
mirroring iam's integration tests). It must run against a DB that has billing's tables — which, after
Component 1, `db-test-bootstrap` provides. Round-trip through the REAL `*Postgres` repo:
`CreateSubscription` → read back → set `status='suspended'`, `suspended_at` → `UpdateSubscription`
→ `GetSubscriptionByTenant` and assert the suspend fields persisted. This exercises the exact columns
`001` was missing, proving the reconciliation and guarding against future drift. (Needs a `tenants`
row first — insert one via `tx`/SQL as the FK parent, following iam's integration-test seeding style.)

## Testing

- **CI migration-validate** already applies billing up+down on a fresh DB — the rewritten `001` +
  the new down must apply and reverse cleanly (verify locally against a scratch container: up → down).
- **New billing integration test** (Component 3) — passes against a migrated `thittam_test`
  (skips locally without `THITTAM_TEST_DSN`; CI's integration-tests job runs it once billing is
  wired into that job's schema — it already is, `ci.yml:258`).
- No unit-test changes (no service code changes).

## Non-goals

- Fixing the parallel `services/billing/db/queries.sql` (sqlc) drift — the running repo uses raw SQL,
  not the generated queries; that's separate dead-code cleanup.
- Adding `reporting`/`audit` to `MIGRATION_DIRS` (pre-existing gaps, unrelated).
- Any `services/billing` Go change — this is migrations + tooling + test only.
- The #126 outbox — resumes on this base after merge.

## Files touched

`migrations/billing/001_create_tables.up.sql` + `.down.sql` (rewrite); `Makefile` (migrate-all +
migrate-down); `scripts/_db-common.sh` (MIGRATION_DIRS); new
`services/billing/db/subscription_integration_test.go`.
