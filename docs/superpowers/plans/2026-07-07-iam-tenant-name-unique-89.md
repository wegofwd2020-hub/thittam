# Finish #89 — tenant-name uniqueness (whitespace-collapse + pre-flight + UUID-naming) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining #89 acceptance gaps left by #91 — collapse internal whitespace in the DB unique index, add a pre-flight duplicate check, and make the `ALREADY_EXISTS` error name the colliding tenant's UUID, all covered by unit + integration tests.

**Architecture:** iam service only. A new migration (018) rebuilds the existing `tenants_name_ci_unique` index (same name) to collapse internal whitespace. A new sqlc query + repo method `FindTenantByNormalizedName` powers a pre-flight check in `Service.CreateTenant`; both the pre-flight and the `23505` race backstop return an error wrapping `ErrTenantNameTaken` with the existing tenant's UUID.

**Tech Stack:** Go 1.22+, sqlc, pgx/v5, PostgreSQL, testify, `pkg/testdb` (integration harness).

## Global Constraints

- iam coverage threshold ≥ 85%.
- SQL only via sqlc / parameterized pgx — no string interpolation.
- Monetary values `NUMERIC(14,2)` / `decimal.Decimal` — N/A here but never `float64`.
- Structured logging via `slog`, no PII/secrets.
- Integration tests use build tag `//go:build integration` and skip when `THITTAM_TEST_DSN` is unset.
- Migration numbering: `migrations/iam/NNN_description.up.sql` / `.down.sql`, zero-padded 3-digit sequential. Highest existing is `017`; the new one is `018`.
- iam change → senior-engineer review + 2 approvals.

---

### Task 1: Migration 018 — collapse internal whitespace in the unique index

**Files:**
- Create: `migrations/iam/018_tenants_name_collapse_whitespace.up.sql`
- Create: `migrations/iam/018_tenants_name_collapse_whitespace.down.sql`
- Modify: `scripts/audit-tenant-name-collisions.sql`
- Test: `services/iam/db/tenant_name_unique_integration_test.go`

**Interfaces:**
- Consumes: existing index name `tenants_name_ci_unique`, existing `insertTenant`/`assertUniqueViolation` test helpers.
- Produces: an index on `regexp_replace(lower(trim(name)), '\s+', ' ', 'g')`, same name `tenants_name_ci_unique`, so `isUniqueViolationOn(err, "tenants_name_ci_unique")` keeps matching.

- [ ] **Step 1: Write the failing integration test case**

Append to `services/iam/db/tenant_name_unique_integration_test.go`:

```go
func TestTenantsNameUnique_InternalWhitespaceCollapsed(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	insertTenant(t, tx, "Acme  Corp") // two spaces

	id := uuid.New()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		id, "Acme Corp", "slug-"+id.String()[:8]) // one space
	assertUniqueViolation(t, err, "tenants_name_ci_unique")
}
```

- [ ] **Step 2: Run test to verify it fails (old index does not collapse)**

Run: `THITTAM_TEST_DSN="$THITTAM_TEST_DSN" go test -tags=integration ./services/iam/db/ -run TestTenantsNameUnique_InternalWhitespaceCollapsed -v`
Expected: FAIL — the second insert succeeds (no `23505`) because the current `lower(trim(name))` index does not collapse the internal double-space, so `assertUniqueViolation` gets a nil error.

- [ ] **Step 3: Write the up migration**

`migrations/iam/018_tenants_name_collapse_whitespace.up.sql`:

```sql
-- 018_tenants_name_collapse_whitespace.up.sql
-- #89: strengthen tenants_name_ci_unique to collapse *internal* whitespace.
--
-- Migration 015 indexed lower(trim(name)) — trim only. That lets
-- "Acme  Corp" (two spaces) and "Acme Corp" (one space) coexist, which
-- violates #89's intent. Rebuild the index (same name, so the repo-layer
-- isUniqueViolationOn helper keeps matching) on a fully normalised
-- expression that also collapses runs of internal whitespace to a single
-- space — matching the application's strings.Fields normalisation.
--
-- Run scripts/audit-tenant-name-collisions.sql against the target database
-- before applying: any pre-existing internal-whitespace duplicates will make
-- CREATE UNIQUE INDEX fail.

DROP INDEX tenants_name_ci_unique;
CREATE UNIQUE INDEX tenants_name_ci_unique
    ON tenants (regexp_replace(lower(trim(name)), '\s+', ' ', 'g'));
```

- [ ] **Step 4: Write the down migration**

`migrations/iam/018_tenants_name_collapse_whitespace.down.sql`:

```sql
-- 018_tenants_name_collapse_whitespace.down.sql
-- Restore the trim-only index from migration 015.
DROP INDEX tenants_name_ci_unique;
CREATE UNIQUE INDEX tenants_name_ci_unique
    ON tenants (lower(trim(name)));
```

- [ ] **Step 5: Update the pre-migration audit script**

Replace the grouping expression in `scripts/audit-tenant-name-collisions.sql` so it catches internal-whitespace duplicates too. The `GROUP BY` / `HAVING` must use the new expression:

```sql
-- Detect tenant-name collisions under the migration-018 normalisation
-- (case-insensitive, trimmed, internal whitespace collapsed). Run before
-- applying 018 — any rows returned would break CREATE UNIQUE INDEX.
SELECT regexp_replace(lower(trim(name)), '\s+', ' ', 'g') AS normalized_name,
       count(*)                                            AS n,
       array_agg(id)                                       AS tenant_ids
FROM tenants
GROUP BY regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
HAVING count(*) > 1;
```

- [ ] **Step 6: Apply migrations to the test DB and re-run the test**

Run: `make migrate-all` (against the `THITTAM_TEST_DSN` database), then
`THITTAM_TEST_DSN="$THITTAM_TEST_DSN" go test -tags=integration ./services/iam/db/ -run 'TestTenantsNameUnique' -v`
Expected: PASS — the new case collides with `23505` on `tenants_name_ci_unique`, and the four pre-existing cases still pass.

- [ ] **Step 7: Commit**

```bash
git add migrations/iam/018_tenants_name_collapse_whitespace.up.sql \
        migrations/iam/018_tenants_name_collapse_whitespace.down.sql \
        scripts/audit-tenant-name-collisions.sql \
        services/iam/db/tenant_name_unique_integration_test.go
git commit -m "feat(iam): collapse internal whitespace in tenants_name_ci_unique (#89)"
```

---

### Task 2: `FindTenantByNormalizedName` — query, repo method, mock

**Files:**
- Modify: `services/iam/db/queries.sql`
- Modify: `services/iam/repository.go:39` (Tenants block)
- Modify: `services/iam/db/postgres.go` (new method near `GetTenant` at :389)
- Modify: `services/iam/service_test.go:43` (mockRepo struct field) and mockRepo methods (~:134)
- Test: `services/iam/db/tenant_find_by_name_integration_test.go` (new)

**Interfaces:**
- Consumes: `dbTenantToDomain(row)` (existing row→`*iam.Tenant` mapper used by `GetTenant`), `p.q` (sqlc `Queries`).
- Produces: `Repository.FindTenantByNormalizedName(ctx context.Context, name string) (*Tenant, error)` — returns `(nil, nil)` when no tenant matches; `(*Tenant, nil)` when one does.

- [ ] **Step 1: Add the sqlc query**

Append to `services/iam/db/queries.sql`:

```sql
-- name: FindTenantByNormalizedName :one
-- Look up a tenant by name under the migration-018 normalisation
-- (case-insensitive, trimmed, internal whitespace collapsed). Uses the
-- tenants_name_ci_unique index. Powers the pre-flight duplicate check.
SELECT * FROM tenants
WHERE regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
    = regexp_replace(lower(trim($1)), '\s+', ' ', 'g')
LIMIT 1;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: a new `FindTenantByNormalizedName(ctx context.Context, name string) (Tenant, error)` in `services/iam/db/*.sql.go`, no errors.

- [ ] **Step 3: Add the method to the Repository interface**

In `services/iam/repository.go`, in the `// Tenants` block (after `GetTenant` at :40), add:

```go
	// FindTenantByNormalizedName returns the tenant whose name matches the
	// given name under case-insensitive, trimmed, internal-whitespace-collapsed
	// comparison (the tenants_name_ci_unique index). Returns (nil, nil) when no
	// tenant matches. Used by CreateTenant's pre-flight duplicate check (#89).
	FindTenantByNormalizedName(ctx context.Context, name string) (*Tenant, error)
```

- [ ] **Step 4: Implement on the Postgres repo**

In `services/iam/db/postgres.go`, after `GetTenant` (ends ~:398), add:

```go
// FindTenantByNormalizedName implements iam.Repository. A no-rows result is
// not an error here — it means the name is free, so return (nil, nil).
func (p *Postgres) FindTenantByNormalizedName(ctx context.Context, name string) (*iam.Tenant, error) {
	row, err := p.q.FindTenantByNormalizedName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("iam/db: find tenant by name: %w", err)
	}
	return dbTenantToDomain(row), nil
}
```

- [ ] **Step 5: Add the mock method (unblocks the package compile)**

In `services/iam/service_test.go`, add a struct field next to `getTenantFn` (~:44):

```go
	findTenantByNormalizedNameFn  func(ctx context.Context, name string) (*Tenant, error)
```

and a method next to `GetTenant` (~:145):

```go
func (m *mockRepo) FindTenantByNormalizedName(ctx context.Context, name string) (*Tenant, error) {
	if m.findTenantByNormalizedNameFn != nil {
		return m.findTenantByNormalizedNameFn(ctx, name)
	}
	return nil, nil
}
```

- [ ] **Step 6: Write the failing integration test**

Create `services/iam/db/tenant_find_by_name_integration_test.go`:

```go
//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestFindTenantByNormalizedName(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	want := insertTenant(t, tx, "Acme  Corp") // two spaces

	repo := iamdb.NewWithTx(tx) // see Step 7 if this constructor differs

	// case + whitespace varied lookup finds the row
	got, err := repo.FindTenantByNormalizedName(context.Background(), "  acme corp ")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want, got.ID)

	// absent name returns (nil, nil)
	missing, err := repo.FindTenantByNormalizedName(context.Background(), "Nobody Inc")
	require.NoError(t, err)
	assert.Nil(t, missing)
}
```

- [ ] **Step 7: Confirm the repo-over-tx constructor name**

Run: `grep -rn "func New" services/iam/db/postgres.go`
If the tx-based constructor is not `NewWithTx`, replace `iamdb.NewWithTx(tx)` in Step 6 with the actual constructor (e.g. `iamdb.New(tx)`). The existing integration tests operate on `tx` directly via raw SQL, so this is the first repo-method integration test — match whatever constructor `postgres.go` exposes that accepts a `pgx` querier.

- [ ] **Step 8: Run the integration test**

Run: `THITTAM_TEST_DSN="$THITTAM_TEST_DSN" go test -tags=integration ./services/iam/db/ -run TestFindTenantByNormalizedName -v`
Expected: PASS.

- [ ] **Step 9: Verify the package still builds (mock satisfies interface)**

Run: `go build ./services/iam/... && go test ./services/iam/ -run TestCreateTenant -short`
Expected: build OK, existing CreateTenant unit tests still PASS.

- [ ] **Step 10: Commit**

```bash
git add services/iam/db/queries.sql services/iam/db/*.sql.go \
        services/iam/repository.go services/iam/db/postgres.go \
        services/iam/service_test.go \
        services/iam/db/tenant_find_by_name_integration_test.go
git commit -m "feat(iam): add FindTenantByNormalizedName repo method (#89)"
```

---

### Task 3: Pre-flight check + UUID-naming error + race backstop

**Files:**
- Modify: `services/iam/errors.go` (add constructor)
- Modify: `services/iam/service.go:437` (pre-flight before insert)
- Modify: `services/iam/db/postgres.go:326` (race path names the UUID)
- Test: `services/iam/service_test.go` (unit: normalization, pre-flight-names-UUID)
- Test: `services/iam/db/tenant_name_unique_integration_test.go` (race)

**Interfaces:**
- Consumes: `Repository.FindTenantByNormalizedName` (Task 2), `ErrTenantNameTaken` sentinel, `isUniqueViolationOn` (postgres.go:341).
- Produces: `tenantNameTakenErr(id uuid.UUID) error` wrapping `ErrTenantNameTaken`; `errors.Is(err, ErrTenantNameTaken)` stays true.

- [ ] **Step 1: Write failing unit test — pre-flight names the UUID**

Add to `services/iam/service_test.go`:

```go
func TestCreateTenant_PreflightDuplicate_NamesUUID(t *testing.T) {
	existing := uuid.New()
	svc := newTestService(&mockRepo{
		findTenantByNormalizedNameFn: func(_ context.Context, _ string) (*Tenant, error) {
			return &Tenant{ID: existing, Name: "Acme Corp"}, nil
		},
	})

	_, err := svc.CreateTenant(context.Background(), &Tenant{
		Name: "  acme   corp ", CountryCode: "US",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTenantNameTaken), "must wrap ErrTenantNameTaken")
	assert.Contains(t, err.Error(), existing.String(), "message must name the colliding UUID")
}
```

Note: match `newTestService(...)` to however other tests construct the service (e.g. `newServiceForTest`); grep `service_test.go` for the existing constructor helper and use it.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./services/iam/ -run TestCreateTenant_PreflightDuplicate_NamesUUID -short -v`
Expected: FAIL — no pre-flight yet, so the mock's default insert path returns nil and `CreateTenant` succeeds instead of erroring.

- [ ] **Step 3: Add the error constructor**

In `services/iam/errors.go`, change the import line and add the constructor below the `var (...)` block:

```go
import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)
```

```go
// tenantNameTakenErr wraps ErrTenantNameTaken with the colliding tenant's
// UUID so the ALREADY_EXISTS surfaced to the caller names the existing
// tenant. errors.Is(err, ErrTenantNameTaken) still reports true, so the
// gRPC mapping in handler.go continues to return codes.AlreadyExists.
func tenantNameTakenErr(id uuid.UUID) error {
	return fmt.Errorf("%w (existing tenant %s)", ErrTenantNameTaken, id)
}
```

- [ ] **Step 4: Add the pre-flight check in Service.CreateTenant**

In `services/iam/service.go`, immediately after the name normalization at :437 (`tenant.Name = strings.Join(strings.Fields(tenant.Name), " ")`), insert:

```go
	// Pre-flight: return a clean ALREADY_EXISTS naming the colliding tenant
	// rather than leaking a raw unique-violation. The DB index is still the
	// backstop for the concurrent-create race (#89).
	if existing, err := s.repo.FindTenantByNormalizedName(ctx, tenant.Name); err != nil {
		return nil, fmt.Errorf("iam: create tenant: %w", err)
	} else if existing != nil {
		return nil, tenantNameTakenErr(existing.ID)
	}
```

- [ ] **Step 5: Run the unit test — expect PASS**

Run: `go test ./services/iam/ -run TestCreateTenant_PreflightDuplicate_NamesUUID -short -v`
Expected: PASS.

- [ ] **Step 6: Write failing unit test — normalization collapses whitespace**

Add to `services/iam/service_test.go`:

```go
func TestCreateTenant_NormalizesName(t *testing.T) {
	var stored string
	svc := newTestService(&mockRepo{
		createTenantFn: func(_ context.Context, t *Tenant) error {
			stored = t.Name
			return nil
		},
	})

	_, err := svc.CreateTenant(context.Background(), &Tenant{
		Name: "  Acme\t Corp   Studios  ", CountryCode: "US",
	})
	require.NoError(t, err)
	assert.Equal(t, "Acme Corp Studios", stored)
}
```

- [ ] **Step 7: Run it — expect PASS (normalization already exists)**

Run: `go test ./services/iam/ -run TestCreateTenant_NormalizesName -short -v`
Expected: PASS — `strings.Join(strings.Fields(...))` already collapses tabs and multiple spaces. This test locks in the behavior (fills the missing-coverage gap); it should be green immediately.

- [ ] **Step 8: Make the race path name the UUID**

In `services/iam/db/postgres.go`, replace the `isUniqueViolationOn` branch at :326-328:

```go
		if isUniqueViolationOn(err, "tenants_name_ci_unique") {
			// Concurrent-create race: the pre-flight missed a same-name insert
			// that landed first. Look up the winner so the caller still gets a
			// UUID-naming ALREADY_EXISTS. Fall back to the bare sentinel if the
			// lookup itself fails.
			if existing, lookupErr := p.FindTenantByNormalizedName(ctx, t.Name); lookupErr == nil && existing != nil {
				return tenantNameTakenErr(existing.ID)
			}
			return iam.ErrTenantNameTaken
		}
```

Note: `tenantNameTakenErr` lives in package `iam` (errors.go). If `postgres.go` is in package `db`, expose the constructor by calling it through the `iam` package — either export it as `iam.TenantNameTakenErr(id)` or add an exported wrapper. Decide in Step 9.

- [ ] **Step 9: Resolve the cross-package constructor visibility**

Run: `head -1 services/iam/db/postgres.go` to confirm the package name.
Since `postgres.go` is `package db` and `errors.go` is `package iam`, rename the constructor to exported `TenantNameTakenErr` in `errors.go` (Step 3) and call `iam.TenantNameTakenErr(existing.ID)` in both the service (same package `iam`, call it unexported-friendly via the exported name too) and postgres.go. Update Step 3, Step 4, and Step 8 call sites to the single exported name `TenantNameTakenErr`. Keep one definition only (DRY).

- [ ] **Step 10: Write the race integration test**

Append to `services/iam/db/tenant_name_unique_integration_test.go`:

```go
func TestCreateTenant_ConcurrentSameName_Race(t *testing.T) {
	pool := testdb.Open(t)
	// NB: not NewTx — concurrent goroutines need independent connections,
	// so use the pool directly and clean up the inserted row(s) at the end.
	const name = "Race Condition Studios"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM tenants WHERE regexp_replace(lower(trim(name)),'\s+',' ','g')
			 = regexp_replace(lower(trim($1)),'\s+',' ','g')`, name)
	})

	const n = 8
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := uuid.New()
			_, err := pool.Exec(context.Background(),
				`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
				 VALUES ($1, $2, $3, 'US', 'USD')`,
				id, name, "slug-"+id.String()[:8])
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	var ok, dup int
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		var pgErr *pgconn.PgError
		if assert.ErrorAs(t, err, &pgErr) && pgErr.Code == "23505" {
			dup++
		}
	}
	assert.Equal(t, 1, ok, "exactly one insert should win")
	assert.Equal(t, n-1, dup, "the rest should be 23505 duplicates")
}
```

Add `"sync"` to the test file's imports.

- [ ] **Step 11: Run the full iam integration + unit suites**

Run: `THITTAM_TEST_DSN="$THITTAM_TEST_DSN" go test -tags=integration ./services/iam/... -v` then `go test ./services/iam/... -short`
Expected: PASS for all, including the four original name-unique cases, the collapse case, the find-by-name case, the race case, and the unit tests.

- [ ] **Step 12: Coverage + vet gate**

Run: `go vet ./services/iam/... && go test ./services/iam/... -short -coverprofile=/tmp/iam.cov && go tool cover -func=/tmp/iam.cov | tail -1`
Expected: vet clean; total coverage ≥ 85%.

- [ ] **Step 13: Commit**

```bash
git add services/iam/errors.go services/iam/service.go services/iam/db/postgres.go \
        services/iam/service_test.go services/iam/db/tenant_name_unique_integration_test.go
git commit -m "feat(iam): pre-flight duplicate check + UUID-naming ALREADY_EXISTS (#89)"
```

---

## Self-Review

**Spec coverage:**
- Whitespace-collapse index → Task 1. ✅
- Pre-migration audit updated → Task 1 Step 5. ✅
- `FindTenantByNormalizedName` plumbing → Task 2. ✅
- Pre-flight check → Task 3 Steps 1–5. ✅
- Error names colliding UUID (common + race) → Task 3 Steps 3–4 (pre-flight) + 8–9 (race). ✅
- Race test → Task 3 Step 10. ✅
- Normalization unit test → Task 3 Steps 6–7. ✅
- Index/query/audit expression identical (`regexp_replace(lower(trim(name)), '\s+', ' ', 'g')`) across Task 1 & 2. ✅
- Out of scope (registration path, fuzzy match) — not in plan, matches spec. ✅
- `tenant-onboarding.md` §7 gap entry — docs-repo change, tracked separately (spec notes this); no task here. ✅

**Type consistency:** `FindTenantByNormalizedName(ctx, name string) (*Tenant, error)` — identical signature in interface (Task 2 Step 3), Postgres impl (Step 4), mock (Step 5), and call sites (Task 3 Steps 4 & 8). Error constructor unified to exported `TenantNameTakenErr(id uuid.UUID) error` in Task 3 Step 9 (resolves the package-boundary ambiguity from Steps 3/8). Index expression string is byte-identical everywhere.

**Placeholder scan:** No TBD/TODO. Two "confirm the exact name" steps (Task 2 Step 7 constructor, Task 3 Step 9 package visibility) are explicit verification steps with a concrete grep + fallback, not deferred work.
