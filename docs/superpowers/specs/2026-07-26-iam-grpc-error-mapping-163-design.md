# Map iam validation sentinels to correct gRPC codes (#163) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issue:** #163 (iam validation sentinels surface as `codes.Internal`, not `InvalidArgument`) — split from #139 slice B
**Branch:** `fix/iam-grpc-error-mapping-163` off `main`
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

`grpcError` (`services/iam/handler.go`) maps many sentinels to codes but leaves several
unmapped — they fall through the `default` arm to `codes.Internal "internal error"`, which
(a) swallows the real reason, (b) makes a client input mistake look like a server fault, and
(c) inflates `Internal`-rate alerting. Map the unmapped iam sentinels to their correct codes.

## Context (grounding facts, `main` @ 08d26ae)

- `grpcError` (`handler.go:781-839`) — `switch` on `errors.Is`; `default` → `status.Error(codes.Internal, "internal error")` (a **fresh** error, so `errors.Is` cannot see the sentinel through it).
- **Unmapped iam sentinels** (audited: every `Err*` in `errors.go` vs the arms in `grpcError`):
  | sentinel | returned by | correct code |
  |---|---|---|
  | `ErrCountryRequired` (`errors.go`) | `SetTenantAddress` (`service.go:506`), `CreateTenant` (`:566`) | `InvalidArgument` |
  | `ErrUnknownCountry` | `SetTenantAddress` (`:510`), `CreateTenant` (`:570`) | `InvalidArgument` |
  | `ErrAmbiguousEmail` | `Login` (email in >1 tenant, must supply tenant_id) | `InvalidArgument` |
  | `ErrPurgeRequestNotApproved` | purge executor (`purge.go`) | `FailedPrecondition` |
  | `ErrNotPlatformAdmin` | *(no call site — dead; grep-confirmed unused)* | `PermissionDenied` (defensive) |
- Existing `InvalidArgument` arm (`:798-801`): `ErrInvalidPlan, ErrRoleNotProjectScoped, ErrHoldUntilInPast`. `FailedPrecondition` arms at `:803-810`. `PermissionDenied` arm (`:828-831`): `auth.ErrTenantSuspended, ErrTenantInactive, ErrAccountDeactivated`.
- **Tests:** `TestHandler_SetTenantAddress_MissingCountry` (`handler_test.go:879`) currently asserts `codes.Internal` and its comment documents the gap. A `grpcError` **table test** exists (`handler_test.go:~1496-1520`): `[]struct{err error; wantCode codes.Code}` iterated via `status.Code(grpcError(tc.err))` — the clean home for the new mappings (covers the dead `ErrNotPlatformAdmin` without an RPC path).

## Design

### `services/iam/handler.go` `grpcError` — add 5 mappings

- Add `ErrCountryRequired`, `ErrUnknownCountry`, `ErrAmbiguousEmail` to the existing
  `InvalidArgument` arm (`:798-801`).
- Add `ErrPurgeRequestNotApproved` to a `FailedPrecondition` arm (`:803-810`).
- Add `ErrNotPlatformAdmin` to the `PermissionDenied` arm (`:828-831`).

All are `errors.Is(err, ErrX)` cases. Because callers wrap some of these
(`fmt.Errorf("%w: %q", ErrUnknownCountry, ...)`), `errors.Is` correctly matches the wrapped
form.

## Testing

- **`grpcError` table test** (`handler_test.go:~1496`): add 5 cases —
  `{ErrCountryRequired, codes.InvalidArgument}`, `{ErrUnknownCountry, codes.InvalidArgument}`,
  `{ErrAmbiguousEmail, codes.InvalidArgument}`, `{ErrPurgeRequestNotApproved, codes.FailedPrecondition}`,
  `{ErrNotPlatformAdmin, codes.PermissionDenied}`. Also add a wrapped case
  (`fmt.Errorf("x: %w", ErrUnknownCountry)` → `InvalidArgument`) to prove `errors.Is` sees
  through the wrap.
- **`TestHandler_SetTenantAddress_MissingCountry`** (`:879`): assert `codes.InvalidArgument`
  (was `Internal`); replace the stale "grpcError has no case" comment.
- Gates: `go test ./services/iam/... -race`; `go vet ./...`; `go build ./...`; `gofmt -l`
  touched files. No proto/sqlc/migration.

## Non-goals

- **Cross-service sweep** — the issue notes the same audit is worth running on the other
  services' `grpcErr`/`grpcError` helpers (expense/budget/project/inventory/ledger/reporting/
  billing/document/notifications). That is a separate, larger effort → **filed as a follow-up
  issue**, not folded here.
- No message-content change for `ErrAmbiguousEmail` (whether it should reveal "email exists in
  multiple tenants" is a separate info-exposure question; this PR only fixes the code).
- No new sentinels; no change to the `default → Internal` behavior for genuinely-internal errors.

## Review weight

`iam` error-mapping (touches Login/tenant-admin surfaces). Security-adjacent but low-risk
(codes only, no logic). Standard review; senior per CLAUDE.md for iam. Whole-branch review.
