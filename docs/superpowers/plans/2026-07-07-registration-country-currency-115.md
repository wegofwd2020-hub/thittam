# Registration country/currency at signup (#115) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `IN`/`INR` stopgap in `pkg/registration` with real country collection at signup and currency derivation, mirroring `services/iam`. Closes #115.

**Architecture:** `pkg/registration` + read-only `pkg/locale`. `RegisterRequest` gains a required `Country` and optional `PrimaryCurrency`; `Validate()` validates country and resolves currency in place; the widened `CreateTenant` threads both into the INSERT (dropping the hardcoded literals).

**Tech Stack:** Go 1.22+, sqlc, pgx/v5, PostgreSQL, testify, `pkg/testdb`, `pkg/locale`.

## Global Constraints

- registration coverage threshold ≥ 75%.
- SQL only via sqlc / parameterized pgx — no string interpolation.
- Mirror iam's model: country required (bare `ErrCountryRequired`), uppercased, `locale.IsKnownCountry` gate (`ErrUnknownCountry`), currency derived via `locale.CurrencyForCountry` when the override is empty.
- Integration tests use build tag `//go:build integration`, skip when `THITTAM_TEST_DSN` unset.
- **Interface-widening check (the #114 lesson):** Task 2 changes the `registration.TenantStore.CreateTenant` signature. EVERY implementer + caller + mock must change atomically or the whole tree fails to compile (build passes locally because it skips `_test.go`). After Task 2, run whole-tree `go vet ./...`. Implementers verified: only `Store`, `TxStore`, `mockTenantStore` — no hidden third.

## Test DB (sandbox)

`make db-test-bootstrap` HANGS here (sudo/askpass) — do NOT run it. `thittam_test` has migrations through 018. Integration DSN:
`postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable`
Run integration: `THITTAM_TEST_DSN="<dsn>" go test -tags=integration ./pkg/registration/... -v`

---

### Task 1: RegisterRequest fields + sentinels + country/currency validation

**Files:**
- Modify: `pkg/registration/request.go` (struct :75, `Validate()` :86-118, add `locale` import)
- Modify: `pkg/registration/errors.go` (2 sentinels)
- Test: `pkg/registration/pipeline_test.go` (`testRequest()` helper, `Validate` table cases, new country cases)

**Interfaces:**
- Produces: `RegisterRequest.Country`, `RegisterRequest.PrimaryCurrency` (both final after `Validate()`); `registration.ErrCountryRequired`, `registration.ErrUnknownCountry`.

- [ ] **Step 1: Add the struct fields**

In `pkg/registration/request.go`, extend `RegisterRequest` (:75-81):

```go
type RegisterRequest struct {
	CompanyName     string
	Email           string
	Password        string
	VerticalID      string
	Plan            string
	Country         string // ISO-3166-1 alpha-2, required
	PrimaryCurrency string // ISO-4217, optional; derived from Country when empty
}
```

- [ ] **Step 2: Add the sentinels**

In `pkg/registration/errors.go`, inside the `var (...)` block (after `ErrTenantNameTaken`):

```go
	// ErrCountryRequired is returned when country is missing at signup.
	ErrCountryRequired = errors.New("registration: country is required")
	// ErrUnknownCountry is returned when the country code has no currency mapping.
	ErrUnknownCountry = errors.New("registration: unknown country")
```

- [ ] **Step 3: Write failing validation tests**

In `pkg/registration/pipeline_test.go`, add:

```go
func TestRegisterRequest_Validate_Country(t *testing.T) {
	t.Parallel()

	t.Run("missing country", func(t *testing.T) {
		r := testRequest()
		r.Country = ""
		assert.ErrorIs(t, r.Validate(), ErrCountryRequired)
	})
	t.Run("unknown country", func(t *testing.T) {
		r := testRequest()
		r.Country = "ZZ"
		assert.ErrorIs(t, r.Validate(), ErrUnknownCountry)
	})
	t.Run("lowercase country accepted and uppercased", func(t *testing.T) {
		r := testRequest()
		r.Country = "in"
		require.NoError(t, r.Validate())
		assert.Equal(t, "IN", r.Country)
	})
	t.Run("currency derived from country", func(t *testing.T) {
		r := testRequest()
		r.Country = "IN"
		r.PrimaryCurrency = ""
		require.NoError(t, r.Validate())
		assert.Equal(t, "INR", r.PrimaryCurrency)
	})
	t.Run("explicit currency override wins", func(t *testing.T) {
		r := testRequest()
		r.Country = "US"
		r.PrimaryCurrency = "eur"
		require.NoError(t, r.Validate())
		assert.Equal(t, "EUR", r.PrimaryCurrency)
	})
}
```

- [ ] **Step 4: Run — verify RED**

Run: `go test ./pkg/registration/ -run TestRegisterRequest_Validate_Country -v`
Expected: FAIL to compile / fail assertions — `Country`/`PrimaryCurrency` and the sentinels don't influence `Validate()` yet (after Steps 1-2 they compile but `Validate` doesn't validate country, so `missing country` returns nil).

- [ ] **Step 5: Add the country/currency block to Validate()**

In `pkg/registration/request.go` `Validate()`, immediately before `return nil` (:118), insert:

```go
	r.Country = strings.ToUpper(strings.TrimSpace(r.Country))
	if r.Country == "" {
		return ErrCountryRequired // bare sentinel, mirroring iam
	}
	if !locale.IsKnownCountry(r.Country) {
		return fmt.Errorf("%w: %q", ErrUnknownCountry, r.Country)
	}
	r.PrimaryCurrency = strings.ToUpper(strings.TrimSpace(r.PrimaryCurrency))
	if r.PrimaryCurrency == "" {
		cur, err := locale.CurrencyForCountry(r.Country)
		if err != nil {
			// Unreachable given IsKnownCountry above; guard against a silent empty currency.
			return fmt.Errorf("%w: %q", ErrUnknownCountry, r.Country)
		}
		r.PrimaryCurrency = cur
	}
```

Add the import `"github.com/wegofwd2020/thittam/pkg/locale"` to request.go.

- [ ] **Step 6: Keep existing tests valid — add Country to testRequest and Validate cases**

In `pkg/registration/pipeline_test.go`, `testRequest()` add `Country: "IN",`:

```go
func testRequest() RegisterRequest {
	return RegisterRequest{
		CompanyName: "Acme Software Pvt. Ltd.",
		Email:       "admin@acmesoftware.com",
		Password:    "securepass123",
		VerticalID:  "software-development",
		Plan:        "professional",
		Country:     "IN",
	}
}
```

Then find every inline `RegisterRequest{...}` literal in the `Validate` table test (grep `RegisterRequest{` in pipeline_test.go — around :253-273) that is expected to PASS validation OR to fail on a *later* field than country, and add `Country: "IN"` so country validation (now near the end of Validate) doesn't pre-empt their intended assertion. Cases that intentionally test an earlier-field failure (empty company name, bad email, etc.) still fail on that earlier check first, so they don't strictly need Country — but add it anyway for clarity unless the case is specifically asserting the country error.

- [ ] **Step 7: Run — verify GREEN + package builds**

Run: `go test ./pkg/registration/ -run 'TestRegisterRequest_Validate' -v && go build ./pkg/registration/...`
Expected: all Validate tests PASS, build clean. (`CreateTenant` signature is unchanged in this task, so the package still compiles.)

- [ ] **Step 8: Commit**

```bash
git add pkg/registration/request.go pkg/registration/errors.go pkg/registration/pipeline_test.go
git commit -m "feat(registration): validate country + derive currency at signup (#115)"
```

---

### Task 2: Thread country/currency through CreateTenant + remove the stopgap

**Files:**
- Modify: `pkg/registration/db/queries.sql` (+ regenerated `*.sql.go`)
- Modify: `pkg/registration/ports.go:14` (CreateTenant signature)
- Modify: `pkg/registration/db/store.go` (`Store.CreateTenant` :46, `TxStore.CreateTenant` ~:151)
- Modify: `pkg/registration/pipeline.go:133`, `pkg/registration/saga.go:277` (call sites)
- Test: `pkg/registration/pipeline_test.go` (`mockTenantStore` signature + a threading assertion)
- Test: `pkg/registration/db/tenant_name_taken_integration_test.go` (CreateTenant call args)

**Interfaces:**
- Consumes: `RegisterRequest.Country`, `RegisterRequest.PrimaryCurrency` (Task 1).
- Produces: `TenantStore.CreateTenant(ctx context.Context, name, slug, plan, country, currency string) (uuid.UUID, error)`.

- [ ] **Step 1: Update the sqlc query (remove the stopgap)**

In `pkg/registration/db/queries.sql`, replace the CreateTenant block (the STOPGAP comment + IN/INR literals) with:

```sql
-- name: CreateTenant :one
INSERT INTO tenants (name, slug, plan, country_code, primary_currency_code)
VALUES ($1, $2, $3, $4, $5) RETURNING *;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: `CreateTenantParams` gains `CountryCode string` + `PrimaryCurrencyCode string`; the generated `CreateTenant` passes all five args. No errors.

- [ ] **Step 3: Widen the port**

In `pkg/registration/ports.go:14`:

```go
	// CreateTenant creates a new tenant and returns its UUID.
	CreateTenant(ctx context.Context, name, slug, plan, country, currency string) (uuid.UUID, error)
```

- [ ] **Step 4: Update Store + TxStore impls**

`pkg/registration/db/store.go` `Store.CreateTenant` (:46):

```go
func (s *Store) CreateTenant(ctx context.Context, name, slug, plan, country, currency string) (uuid.UUID, error) {
	t, err := s.q.CreateTenant(ctx, CreateTenantParams{
		Name: name, Slug: slug, Plan: plan,
		CountryCode: country, PrimaryCurrencyCode: currency,
	})
	if err != nil {
		if isUniqueViolationOn(err, "tenants_name_ci_unique") {
			return uuid.Nil, registration.ErrTenantNameTaken
		}
		return uuid.Nil, fmt.Errorf("create tenant: %w", err)
	}
	return t.ID, nil
}
```

Apply the identical signature + params change to `TxStore.CreateTenant` (~:151), preserving its existing body structure.

- [ ] **Step 5: Update the two call sites**

`pkg/registration/pipeline.go:133`:

```go
	tenantID, err := p.tenants.CreateTenant(ctx, req.CompanyName, slug, req.Plan, req.Country, req.PrimaryCurrency)
```

`pkg/registration/saga.go:277`:

```go
	tenantID, err := o.pipeline.tenants.CreateTenant(ctx, req.CompanyName, slug, req.Plan, req.Country, req.PrimaryCurrency)
```

- [ ] **Step 6: Update the mock (signature) + add a threading assertion**

In `pkg/registration/pipeline_test.go`, widen `mockTenantStore.createTenantFn` field and method:

```go
	createTenantFn func(ctx context.Context, name, slug, plan, country, currency string) (uuid.UUID, error)
```

```go
func (m *mockTenantStore) CreateTenant(ctx context.Context, name, slug, plan, country, currency string) (uuid.UUID, error) {
	if m.createTenantFn != nil {
		return m.createTenantFn(ctx, name, slug, plan, country, currency)
	}
	return uuid.New(), nil
}
```

Add a test asserting the resolved country/currency reach CreateTenant:

```go
func TestPipeline_CreateTenant_ReceivesCountryCurrency(t *testing.T) {
	t.Parallel()
	var gotCountry, gotCurrency string
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			createTenantFn: func(_ context.Context, _, _, _, country, currency string) (uuid.UUID, error) {
				gotCountry, gotCurrency = country, currency
				return uuid.New(), nil
			},
		}
	})

	_, err := p.Run(context.Background(), testRequest()) // Country "IN" → currency "INR"
	require.NoError(t, err)
	assert.Equal(t, "IN", gotCountry)
	assert.Equal(t, "INR", gotCurrency)
}
```

- [ ] **Step 7: Update the integration test's CreateTenant calls**

In `pkg/registration/db/tenant_name_taken_integration_test.go`, update both `store.CreateTenant(...)` calls (~:28, :32) to pass country + currency:

```go
	_, err := store.CreateTenant(context.Background(), name, "reg-parity-studios", "starter", "IN", "INR")
```
```go
	_, err = store.CreateTenant(context.Background(), "reg parity studios", "reg-parity-studios-2", "starter", "IN", "INR")
```
(Match the existing variable names/slugs already in that test; only append the two new args.)

- [ ] **Step 8: Whole-tree vet + unit tests (interface-widening check)**

Run: `go vet ./...`
then: `go test ./pkg/registration/... -short`
Expected: `go vet ./...` clean across the WHOLE tree (catches any missed CreateTenant caller/implementer); unit tests PASS.

- [ ] **Step 9: Integration test**

Run: `THITTAM_TEST_DSN="postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" go test -tags=integration ./pkg/registration/db/ -run TestStore_CreateTenant_NameTaken -v`
Expected: PASS (now inserts real country/currency instead of the removed literals).

- [ ] **Step 10: Coverage gate**

Run: `go test ./pkg/registration/ -short -coverprofile=/tmp/reg.cov && go tool cover -func=/tmp/reg.cov | tail -1`
Expected: coverage ≥ 75%.

- [ ] **Step 11: Commit**

```bash
git add pkg/registration/db/queries.sql pkg/registration/db/queries.sql.go \
        pkg/registration/ports.go pkg/registration/db/store.go \
        pkg/registration/pipeline.go pkg/registration/saga.go \
        pkg/registration/pipeline_test.go \
        pkg/registration/db/tenant_name_taken_integration_test.go
git commit -m "feat(registration): thread country/currency into CreateTenant, drop IN/INR stopgap (#115)"
```

---

## Self-Review

**Spec coverage:**
- Required `Country` + optional `PrimaryCurrency` fields → Task 1 Step 1. ✅
- `ErrCountryRequired` / `ErrUnknownCountry` sentinels → Task 1 Step 2. ✅
- Validate: reject missing/unknown, uppercase, derive/override → Task 1 Step 5. ✅
- Widen CreateTenant through port/Store/TxStore/pipeline/saga/mock → Task 2 Steps 3-6. ✅
- Remove IN/INR literals + STOPGAP comment → Task 2 Step 1. ✅
- Whole-tree `go vet ./...` → Task 2 Step 8. ✅
- Unit tests (required, unknown, case, derive, override, threading) → Task 1 Step 3 + Task 2 Step 6. ✅
- Integration test updated → Task 2 Step 7. ✅
- Coverage ≥75% → Task 2 Step 10. ✅
- Out of scope (currency-set validation, production wiring) — not in plan. ✅

**Type consistency:** `CreateTenant(ctx, name, slug, plan, country, currency string) (uuid.UUID, error)` identical across port (Task 2 Step 3), Store/TxStore (Step 4), both call sites (Step 5), and mock (Step 6). `CreateTenantParams` field names `CountryCode`/`PrimaryCurrencyCode` match the generated sqlc struct (from the same `tenants` columns iam uses). Sentinels `ErrCountryRequired`/`ErrUnknownCountry` defined in Task 1 Step 2, consumed by Task 1 Step 5 and the Task 1 Step 3 tests.

**Placeholder scan:** No TBD/TODO. Task 1 Step 6 ("grep the Validate cases and add Country") is an explicit, grep-backed edit with a stated rule (add to cases expected to pass or fail-later-than-country), not deferred work. Task 2 Step 4's "preserve TxStore's existing body structure" points at the sibling `Store` impl shown in full just above.

**Interface-widening (the #114 lesson):** Task 2 changes the CreateTenant signature; Step 8 runs whole-tree `go vet ./...`. Implementers verified as only Store/TxStore/mockTenantStore (see [[reference_iam_repository_implementers]] for the analogous iam case). The atomic signature change is deliberately one task so the tree never sits in a half-migrated, non-compiling state between reviews.
