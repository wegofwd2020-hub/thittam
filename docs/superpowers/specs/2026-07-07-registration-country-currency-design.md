# Design: registration country/currency at signup (#115)

**Closes:** #115 — remove the `IN`/`INR` stopgap; collect country at signup and derive currency, mirroring iam.
**Follows:** the registration tenant-name parity PR (#116, merged) which introduced the stopgap.
**Date:** 2026-07-07
**Scope:** `pkg/registration` (+ read-only use of `pkg/locale`)
**Branch:** `feat/registration-country-currency-115`

## Context

Migration 014 made `tenants.country_code` / `tenants.primary_currency_code` `NOT NULL`
with no DB default. The tenant-name parity work found that `pkg/registration`'s
`CreateTenant` never set these, so real inserts 23502'd — and shipped a stopgap that
hardcodes `'IN','INR'` in the sqlc query (`pkg/registration/db/queries.sql`, marked
`STOPGAP — TODO(#115)`). That means every registration-created tenant would be
India/INR regardless of actual country once registration is wired to a live signup.

This replaces the stopgap with real country collection, mirroring
`services/iam/Service.CreateTenant` (`service.go` ~:465-486): **country required,
currency derived from country, with an optional explicit currency override**.

`pkg/locale` is a pure in-memory map (no DB) exposing:
- `CurrencyForCountry(countryCode string) (string, error)` — case-insensitive, returns a
  formatted error for unknown/malformed codes.
- `IsKnownCountry(countryCode string) bool`.

Registration is still not wired to a production handler (no proto/HTTP layer to
update); this is a library-level change. No `RegisterRequest` is constructed outside
`pkg/registration` (verified).

## Decisions (confirmed)

- **Country is required** at signup — reject empty/unknown, mirroring iam. No silent default.
- **Currency override** — add an optional `PrimaryCurrency` field; derive from country when empty, mirroring iam and the locale contract (multi-currency countries like PR→USD).

## Component 1 — RegisterRequest fields + sentinels

`pkg/registration/request.go` — add to the `RegisterRequest` struct (:75):

```go
	Country         string // ISO-3166-1 alpha-2, required
	PrimaryCurrency string // ISO-4217, optional; derived from Country when empty
```

`pkg/registration/errors.go` — add sentinels (mirror iam's messages, registration-prefixed):

```go
	// ErrCountryRequired is returned when country is missing at signup.
	ErrCountryRequired = errors.New("registration: country is required")
	// ErrUnknownCountry is returned when the country code has no currency mapping.
	ErrUnknownCountry = errors.New("registration: unknown country")
```

## Component 2 — Validation + currency resolution in Validate()

`pkg/registration/request.go` `Validate()` — after the existing field guards (plan
check), add country validation and currency resolution so both fields are final by
the time the pipeline/saga read them (`Validate()` already normalizes other fields
in place, and is called at the top of both `pipeline.Run` and the orchestrator's
`Start`):

```go
	r.Country = strings.ToUpper(strings.TrimSpace(r.Country))
	if r.Country == "" {
		return ErrCountryRequired // bare sentinel, mirroring iam's ErrCountryRequired return
	}
	if !locale.IsKnownCountry(r.Country) {
		return fmt.Errorf("%w: %q", ErrUnknownCountry, r.Country)
	}
	r.PrimaryCurrency = strings.ToUpper(strings.TrimSpace(r.PrimaryCurrency))
	if r.PrimaryCurrency == "" {
		// Derive from country. IsKnownCountry above guarantees this won't error,
		// but propagate defensively to avoid a silent empty currency.
		cur, err := locale.CurrencyForCountry(r.Country)
		if err != nil {
			return fmt.Errorf("%w: %q", ErrUnknownCountry, r.Country)
		}
		r.PrimaryCurrency = cur
	}
```

Add the `pkg/locale` import to request.go. (No new length/format CHECK needed here —
`locale.IsKnownCountry` gates country, and derived currencies are always valid
3-letter codes; an explicit override is uppercased and will be rejected by the DB
CHECK `^[A-Z]{3}$` if malformed. See "Open consideration" below.)

## Component 3 — Widen CreateTenant through the stack

Signature becomes `CreateTenant(ctx, name, slug, plan, country, currency string) (uuid.UUID, error)`.
Every site (blast radius verified — no hidden implementer; #114 lesson):

- **Port** — `pkg/registration/ports.go:14` (`TenantStore.CreateTenant`).
- **Impls** — `pkg/registration/db/store.go`: `Store.CreateTenant` (:46) and
  `TxStore.CreateTenant` (:151), each building
  `CreateTenantParams{Name, Slug, Plan, CountryCode: country, PrimaryCurrencyCode: currency}`.
- **Call sites** — `pipeline.go:133` and `saga.go:277`:
  `CreateTenant(ctx, req.CompanyName, slug, req.Plan, req.Country, req.PrimaryCurrency)`.
- **Mock** — `pkg/registration/pipeline_test.go` `mockTenantStore.createTenantFn`
  field (:27) + method — widen the signature (saga_test reuses this mock).

After wiring, run whole-tree `go vet ./...` (the interface-widening check).

## Component 4 — sqlc query: remove the stopgap

`pkg/registration/db/queries.sql` — replace the stopgap CreateTenant with:

```sql
-- name: CreateTenant :one
INSERT INTO tenants (name, slug, plan, country_code, primary_currency_code)
VALUES ($1, $2, $3, $4, $5) RETURNING *;
```

Drop the `'IN','INR'` literals and the `STOPGAP — TODO(#115)` comment. `sqlc generate`
regenerates `CreateTenantParams` with `CountryCode` + `PrimaryCurrencyCode`.

## Component 5 — Tests

**Unit** (`pkg/registration/*_test.go`):

- `testRequest()` helper — add `Country: "IN"` so the canonical request stays valid.
  (Its derived currency becomes INR via Validate.)
- `Validate` cases: missing country → `ErrCountryRequired`; unknown country
  (e.g. `"ZZ"`) → `ErrUnknownCountry`; lowercase country (`"in"`) accepted +
  uppercased; derived currency (Country `"IN"`, no override → `PrimaryCurrency == "INR"`);
  explicit override (Country `"US"`, `PrimaryCurrency "eur"` → `"EUR"` wins, not USD).
- `mockTenantStore.createTenantFn` widened; existing pipeline/saga tests updated to
  the new signature. Add an assertion (in a pipeline test) that `CreateTenant`
  receives the resolved country + currency (e.g. `"IN"`, `"INR"`).

**Integration** (`pkg/registration/db/tenant_name_taken_integration_test.go`): update
the two `store.CreateTenant(...)` calls to pass country + currency (e.g. `"IN", "INR"`).
No behavior change to that test's assertions.

## Acceptance criteria

- [ ] `RegisterRequest` has required `Country` + optional `PrimaryCurrency`.
- [ ] `Validate()` rejects missing/unknown country (`ErrCountryRequired`/`ErrUnknownCountry`), uppercases country, and resolves currency (derive when override empty).
- [ ] `CreateTenant` threads country + currency into the INSERT; the `IN`/`INR` literals and STOPGAP comment are gone.
- [ ] Whole-tree `go vet ./...` clean (interface widening).
- [ ] Unit tests: country required, unknown country, case-normalization, derivation, override.
- [ ] Integration test updated + passing.
- [ ] registration coverage ≥ 75%. Closes #115.

## Open consideration

An explicit `PrimaryCurrency` override is only length/case-normalized here, not
checked against a known-currency set (`pkg/locale` has no currency validator). A
malformed override is caught by the DB CHECK `primary_currency_code ~ '^[A-Z]{3}$'`
at insert (surfaces as a wrapped error, not a clean sentinel). iam has the same
limitation. Adding a currency validator to `pkg/locale` is out of scope; noted for a
possible follow-up if signup UX needs a clean pre-insert rejection.

## Out of scope

- Wiring registration to a production handler + HTTP/proto mapping (still no caller).
- Currency-set validation in `pkg/locale`.

## Files touched

- `pkg/registration/request.go` (fields + Validate + locale import)
- `pkg/registration/errors.go` (2 sentinels)
- `pkg/registration/ports.go` (CreateTenant signature)
- `pkg/registration/db/store.go` (Store + TxStore CreateTenant params)
- `pkg/registration/db/queries.sql` (+ regenerated sqlc)
- `pkg/registration/pipeline.go` / `saga.go` (call sites)
- `pkg/registration/pipeline_test.go` / `saga_test.go` (mock signature, testRequest, Validate cases)
- `pkg/registration/db/tenant_name_taken_integration_test.go` (CreateTenant call args)

Review: shared-tenancy onboarding change — senior review advisable.
