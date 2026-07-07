# Registration tenant-name uniqueness parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `pkg/registration` to parity with iam's tenant-name uniqueness — normalize `CompanyName`, pre-flight the normalized name, and map the shared `tenants_name_ci_unique` 23505 to a new `registration.ErrTenantNameTaken` sentinel.

**Architecture:** `pkg/registration` only. Registration writes to the same `tenants` table + `tenants_name_ci_unique` index (from `migrations/iam/`) that iam uses; the DB backstop already applies. This adds the missing normalization, a pre-flight existence check (mirroring the existing email/slug pre-flights), and 23505 classification — all returning a clean typed sentinel. HTTP/409 mapping is out of scope (registration has no production handler yet).

**Tech Stack:** Go 1.22+, sqlc, pgx/v5 (+ pgconn for error classification), PostgreSQL, testify, `pkg/testdb`.

## Global Constraints

- registration coverage threshold ≥ 75%.
- SQL only via sqlc / parameterized pgx — no string interpolation.
- Normalization expression must match the DB index exactly: `regexp_replace(lower(trim(name)), '\s+', ' ', 'g')`; the Go equivalent is `strings.Join(strings.Fields(name), " ")` (collapse internal whitespace + trim, preserve case).
- Mirror iam's clean-error model, NOT slug's silent auto-suffix.
- Integration tests use build tag `//go:build integration` and skip when `THITTAM_TEST_DSN` is unset.
- **Interface-widening check (the #114 lesson):** Task 2 adds a method to the `registration.TenantStore` interface. EVERY implementer must get it or the whole tree fails to compile in CI (build passes locally because it skips `_test.go`). After Task 2, run whole-tree `go vet ./...`, not a package-scoped test. Find implementers with `grep -rn "TenantStore\|TenantExistsBySlug" --include=*.go pkg/registration`.

## Test DB (sandbox)

`make db-test-bootstrap` HANGS here (sudo/askpass, no TTY) — do NOT run it. `thittam_test` already has migrations through 018 applied. Integration DSN:
`postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable`
Run integration tests: `THITTAM_TEST_DSN="<dsn>" go test -tags=integration ./pkg/registration/... -v`

---

### Task 1: Normalize CompanyName + add the sentinel

**Files:**
- Modify: `pkg/registration/request.go:87`
- Modify: `pkg/registration/errors.go` (add sentinel to the `var (...)` block)
- Test: `pkg/registration/pipeline_test.go` (`TestRegisterRequest_Validate`, ~:510)

**Interfaces:**
- Produces: `registration.ErrTenantNameTaken` (consumed by Tasks 2 & 3); normalized `CompanyName` in `Validate()`.

- [ ] **Step 1: Add a failing normalization case to `TestRegisterRequest_Validate`**

Locate `TestRegisterRequest_Validate` (`pipeline_test.go:510`). Add a subtest/case asserting internal-whitespace collapse. If it is table-driven, add a row; otherwise add:

```go
func TestRegisterRequest_Validate_CollapsesCompanyNameWhitespace(t *testing.T) {
	t.Parallel()
	r := testRequest()
	r.CompanyName = "  Acme\t Corp   Studios  "
	require.NoError(t, r.Validate())
	assert.Equal(t, "Acme Corp Studios", r.CompanyName)
}
```

- [ ] **Step 2: Run it — verify RED**

Run: `go test ./pkg/registration/ -run TestRegisterRequest_Validate_CollapsesCompanyNameWhitespace -v`
Expected: FAIL — current `strings.TrimSpace` leaves internal double/tab whitespace, so `CompanyName` is `"Acme\t Corp   Studios"`, not collapsed.

- [ ] **Step 3: Normalize in `Validate()`**

In `pkg/registration/request.go`, replace line 87:

```go
r.CompanyName = strings.TrimSpace(r.CompanyName)
```

with:

```go
// Collapse internal whitespace runs and trim, matching iam's canonical form
// and the tenants_name_ci_unique index (regexp_replace(lower(trim(name)),'\s+',' ','g')).
r.CompanyName = strings.Join(strings.Fields(r.CompanyName), " ")
```

(`strings` is already imported in request.go.)

- [ ] **Step 4: Add the sentinel**

In `pkg/registration/errors.go`, inside the `var (...)` block (after `ErrEmailTaken` at :13), add:

```go
	// ErrTenantNameTaken is returned when a tenant already exists with the same
	// company name (case-insensitive, whitespace-collapsed). Enforced by the
	// shared tenants_name_ci_unique index; mirrors iam.ErrTenantNameTaken.
	ErrTenantNameTaken = errors.New("registration: company name already taken")
```

- [ ] **Step 5: Run to verify GREEN + package builds**

Run: `go test ./pkg/registration/ -run TestRegisterRequest_Validate -v && go build ./pkg/registration/...`
Expected: PASS; build clean. (The sentinel is defined but not yet consumed — a package-level var, no unused error.)

- [ ] **Step 6: Commit**

```bash
git add pkg/registration/request.go pkg/registration/errors.go pkg/registration/pipeline_test.go
git commit -m "feat(registration): normalize CompanyName + add ErrTenantNameTaken (#89 follow-up)"
```

---

### Task 2: Pre-flight normalized-name check

**Files:**
- Modify: `pkg/registration/db/queries.sql` (+ regenerated `*.sql.go`)
- Modify: `pkg/registration/ports.go:21` (TenantStore interface)
- Modify: `pkg/registration/db/store.go` (impl on `Store` and `TxStore`)
- Modify: `pkg/registration/pipeline.go:91` (pre-flight)
- Modify: `pkg/registration/saga.go:262` (pre-flight)
- Test: `pkg/registration/pipeline_test.go` (mock field + `TestPipeline_TenantNameTaken`), `pkg/registration/saga_test.go`

**Interfaces:**
- Consumes: `ErrTenantNameTaken` (Task 1).
- Produces: `TenantStore.TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error)` — true if a tenant already exists under normalized comparison.

- [ ] **Step 1: Add the sqlc query**

In `pkg/registration/db/queries.sql`, after `TenantExistsBySlug` (:4-5), add:

```sql
-- name: TenantExistsByNormalizedName :one
SELECT EXISTS (
    SELECT 1 FROM tenants
    WHERE regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
        = regexp_replace(lower(trim($1)), '\s+', ' ', 'g')
);
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: `TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error)` appears in `pkg/registration/db/queries.sql.go`, no errors.

- [ ] **Step 3: Add the port method**

In `pkg/registration/ports.go`, in the `TenantStore` interface after `TenantExistsBySlug` (:21):

```go
	// TenantExistsByNormalizedName returns true if a tenant already exists with
	// this name under case-insensitive, trimmed, whitespace-collapsed comparison.
	TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error)
```

- [ ] **Step 4: Implement on `Store` and `TxStore`**

In `pkg/registration/db/store.go`, add next to each `CreateTenant`:

```go
func (s *Store) TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error) {
	return s.q.TenantExistsByNormalizedName(ctx, name)
}
```

```go
func (ts TxStore) TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error) {
	return ts.q.TenantExistsByNormalizedName(ctx, name)
}
```

- [ ] **Step 5: Find and update ALL other TenantStore implementers**

Run: `grep -rn "TenantExistsBySlug" --include=*.go pkg/registration`
For every type that implements `TenantStore` (at minimum `mockTenantStore` in `pipeline_test.go:19`, plus any mock in `saga_test.go`), add the method. For `mockTenantStore` add the field and method:

```go
	tenantExistsByNormalizedNameFn func(ctx context.Context, name string) (bool, error)
```

```go
func (m *mockTenantStore) TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error) {
	if m.tenantExistsByNormalizedNameFn != nil {
		return m.tenantExistsByNormalizedNameFn(ctx, name)
	}
	return false, nil
}
```

- [ ] **Step 6: Wire the pipeline pre-flight**

In `pkg/registration/pipeline.go`, after the email check (ends :91), before slug generation (:93), add:

```go
	// Check tenant-name uniqueness (normalized). Unlike slug (which silently
	// auto-suffixes on collision), a duplicate name is a clean rejection.
	nameTaken, err := p.tenants.TenantExistsByNormalizedName(ctx, req.CompanyName)
	if err != nil {
		return nil, fmt.Errorf("check tenant name: %w", err)
	}
	if nameTaken {
		return nil, ErrTenantNameTaken
	}
```

`req.CompanyName` is already normalized by `Validate()`. (Confirm `Run` calls `req.Validate()` before this point — grep `Validate()` in pipeline.go; it is called at the top of `Run`.)

- [ ] **Step 7: Wire the saga pre-flight**

In `pkg/registration/saga.go`, in the Step 1 block after slug handling (:269), before `CreateTenant` (:271), add:

```go
		if taken, err := o.pipeline.tenants.TenantExistsByNormalizedName(ctx, req.CompanyName); err != nil {
			return saga, o.failSaga(ctx, saga, StepCreateTenant, fmt.Errorf("check tenant name: %w", err))
		} else if taken {
			return saga, o.failSaga(ctx, saga, StepCreateTenant, ErrTenantNameTaken)
		}
```

- [ ] **Step 8: Write the failing pipeline test**

In `pkg/registration/pipeline_test.go`, mirror `TestPipeline_EmailTaken` (:314):

```go
func TestPipeline_TenantNameTaken(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			tenantExistsByNormalizedNameFn: func(ctx context.Context, name string) (bool, error) {
				return true, nil
			},
		}
	})

	_, err := p.Run(context.Background(), testRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTenantNameTaken)
}
```

- [ ] **Step 9: Write the failing saga test**

In `pkg/registration/saga_test.go`, mirror the orchestrator's existing conflict-path test (grep for `ErrEmailTaken`/`newTestOrchestrator` to match the harness). Inject the pipeline's tenant store with `tenantExistsByNormalizedNameFn` returning `(true, nil)` and assert the run fails with `errors.Is(err, ErrTenantNameTaken)`. Use the same orchestrator construction the sibling tests use; do not invent a new harness.

- [ ] **Step 10: Run unit tests + WHOLE-TREE vet (interface-widening check)**

Run: `go test ./pkg/registration/ -run 'TestPipeline_TenantNameTaken|TestRegisterRequest_Validate|TestSaga' -v`
then: `go vet ./...`
Expected: tests PASS; `go vet ./...` clean across the WHOLE tree (this catches any `TenantStore` implementer missed in Step 5 — a package-scoped test would not).

- [ ] **Step 11: Commit**

```bash
git add pkg/registration/db/queries.sql pkg/registration/db/queries.sql.go \
        pkg/registration/ports.go pkg/registration/db/store.go \
        pkg/registration/pipeline.go pkg/registration/saga.go \
        pkg/registration/pipeline_test.go pkg/registration/saga_test.go
git commit -m "feat(registration): pre-flight normalized tenant-name check (#89 follow-up)"
```

---

### Task 3: 23505 backstop in the store + integration test

**Files:**
- Modify: `pkg/registration/db/store.go` (helper + `CreateTenant` on `Store` and `TxStore`, imports)
- Test: `pkg/registration/db/store_test.go` (unit: `isUniqueViolationOn`)
- Test: `pkg/registration/db/tenant_name_taken_integration_test.go` (new)

**Interfaces:**
- Consumes: `registration.ErrTenantNameTaken` (Task 1), `TenantExistsByNormalizedName` (Task 2).
- Produces: `Store`/`TxStore` `CreateTenant` returns `ErrTenantNameTaken` on a `tenants_name_ci_unique` 23505.

- [ ] **Step 1: Write the failing helper unit test**

In `pkg/registration/db/store_test.go`:

```go
func TestIsUniqueViolationOn(t *testing.T) {
	t.Parallel()
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "tenants_name_ci_unique"}
	assert.True(t, isUniqueViolationOn(pgErr, "tenants_name_ci_unique"))
	assert.False(t, isUniqueViolationOn(pgErr, "other_index"))
	assert.False(t, isUniqueViolationOn(&pgconn.PgError{Code: "23503", ConstraintName: "tenants_name_ci_unique"}, "tenants_name_ci_unique"))
	assert.False(t, isUniqueViolationOn(errors.New("plain"), "tenants_name_ci_unique"))
}
```

Add imports to store_test.go as needed: `errors`, `github.com/jackc/pgx/v5/pgconn`, `github.com/stretchr/testify/assert`.

- [ ] **Step 2: Run — verify RED**

Run: `go test ./pkg/registration/db/ -run TestIsUniqueViolationOn -v`
Expected: FAIL to compile — `isUniqueViolationOn` undefined.

- [ ] **Step 3: Add the helper + map the error on both stores**

In `pkg/registration/db/store.go`, add `"github.com/jackc/pgx/v5/pgconn"` to imports (`errors` is already imported at :6). Add the helper:

```go
// isUniqueViolationOn reports whether err is a Postgres unique_violation (23505)
// on the named constraint/index. Parameterized for symmetry with iam's helper.
func isUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
```

Update `Store.CreateTenant` (:35-41):

```go
func (s *Store) CreateTenant(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
	t, err := s.q.CreateTenant(ctx, CreateTenantParams{Name: name, Slug: slug, Plan: plan})
	if err != nil {
		if isUniqueViolationOn(err, "tenants_name_ci_unique") {
			return uuid.Nil, registration.ErrTenantNameTaken
		}
		return uuid.Nil, fmt.Errorf("create tenant: %w", err)
	}
	return t.ID, nil
}
```

Apply the identical mapping to `TxStore.CreateTenant` (:133-139). (`registration` is already imported in store.go at :12.)

- [ ] **Step 4: Run the helper test — GREEN**

Run: `go test ./pkg/registration/db/ -run TestIsUniqueViolationOn -v`
Expected: PASS.

- [ ] **Step 5: Write the integration test**

Create `pkg/registration/db/tenant_name_taken_integration_test.go`:

```go
//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/registration"
	regdb "github.com/wegofwd2020/thittam/pkg/registration/db"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestStore_CreateTenant_NameTaken(t *testing.T) {
	pool := testdb.Open(t)
	store := regdb.NewStore(pool, nil) // vq nil: tenant methods don't use it

	const name = "Reg Parity  Studios" // double internal space
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM tenants WHERE regexp_replace(lower(trim(name)),'\s+',' ','g')
			 = regexp_replace(lower(trim($1)),'\s+',' ','g')`, name)
	})

	_, err := store.CreateTenant(context.Background(), name, "reg-parity-studios", "starter")
	require.NoError(t, err)

	// Case + whitespace varied duplicate → clean sentinel, not a raw pg error.
	_, err = store.CreateTenant(context.Background(), "reg parity studios", "reg-parity-studios-2", "starter")
	require.Error(t, err)
	assert.ErrorIs(t, err, registration.ErrTenantNameTaken)

	// Pre-flight existence check agrees.
	exists, err := store.TenantExistsByNormalizedName(context.Background(), "  REG PARITY studios ")
	require.NoError(t, err)
	assert.True(t, exists)

	absent, err := store.TenantExistsByNormalizedName(context.Background(), "Nobody Reg Inc")
	require.NoError(t, err)
	assert.False(t, absent)
}
```

Confirm `regdb.NewStore(pool, nil)` matches the real constructor (`grep -n "func NewStore" pkg/registration/db/store.go` — it is `NewStore(db *pgxpool.Pool, vq *verticaldb.Queries)`). If tenant `CreateTenant` dereferences `vq`, pass a real `verticaldb.New(pool)` instead of nil — but it does not (vq is only used by vertical methods).

- [ ] **Step 6: Run the integration test**

Run: `THITTAM_TEST_DSN="postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" go test -tags=integration ./pkg/registration/db/ -run TestStore_CreateTenant_NameTaken -v`
Expected: PASS.

- [ ] **Step 7: Full verification (whole tree + coverage)**

Run: `go vet ./... && go test ./pkg/registration/... -short && go test ./pkg/registration/ -short -coverprofile=/tmp/reg.cov && go tool cover -func=/tmp/reg.cov | tail -1`
Expected: vet clean; unit tests pass; coverage ≥ 75%.

- [ ] **Step 8: Commit**

```bash
git add pkg/registration/db/store.go pkg/registration/db/store_test.go \
        pkg/registration/db/tenant_name_taken_integration_test.go
git commit -m "feat(registration): map tenants_name_ci_unique 23505 to ErrTenantNameTaken (#89 follow-up)"
```

---

## Self-Review

**Spec coverage:**
- Normalize CompanyName → Task 1 Steps 1-3. ✅
- `ErrTenantNameTaken` sentinel → Task 1 Step 4. ✅
- Pre-flight (query + port + Store/TxStore impl + pipeline + saga) → Task 2. ✅
- 23505 backstop on Store + TxStore → Task 3 Step 3. ✅
- Unit tests (normalization, pipeline pre-flight, saga pre-flight, isUniqueViolationOn) → Task 1 Step 1, Task 2 Steps 8-9, Task 3 Step 1. ✅
- Integration test (real store duplicate → sentinel) → Task 3 Step 5. ✅
- Coverage ≥75% → Task 3 Step 7. ✅
- Out of scope (HTTP 409, UUID-in-error) — not in plan, matches spec. ✅

**Type consistency:** `TenantExistsByNormalizedName(ctx, name string) (bool, error)` identical in query (regen), port (Task 2 Step 3), Store/TxStore (Step 4), mock (Step 5), and both call sites (Steps 6-7). `isUniqueViolationOn(err error, constraint string) bool` matches iam's helper and its call `isUniqueViolationOn(err, "tenants_name_ci_unique")`. Normalization expression byte-identical (`regexp_replace(lower(trim(name)),'\s+',' ','g')` / `strings.Join(strings.Fields(name)," ")`) everywhere.

**Placeholder scan:** No TBD/TODO. The "confirm Validate() is called in Run" (Task 2 Step 6) and "confirm NewStore constructor" (Task 3 Step 5) are explicit grep-backed verification steps, not deferred work. Task 2 Step 9's saga test says "match the sibling harness" rather than pasting exact code because the saga test-harness shape must be read from `saga_test.go` at implementation time — the sibling `ErrEmailTaken`-style test is named as the template.

**Interface-widening (the #114 lesson):** Task 2 Step 5 explicitly greps for all `TenantStore` implementers, and Step 10 runs whole-tree `go vet ./...`. See the reference memory on `iam.Repository` implementers — the same failure mode applies to any widely-implemented interface.
