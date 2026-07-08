# Design: SetTenantRetention operator override (#119)

**Issue:** #119 (carved from #90 / retention-lifecycle sequence). **Date:** 2026-07-08
**Scope:** iam only — one new gRPC RPC + repo write path + audit. No migration.
**Branch:** `feat/set-tenant-retention-119` (to be created)
**Depends on:** #118 (merged, `b607d7c`) — the retention clock this override adjusts.

## Context

The retention sweeper (`cmd/retention-sweeper/`, daily CronJob) advances a suspended tenant
`suspended → grace → deactivated → purge_eligible` on a clock **derived at query time** from
`suspended_at`/`deactivated_at` plus fixed intervals (30/90/180 days —
`services/iam/lifecycle.go:16-30`). Operators need a manual override to **hold** a tenant on
that clock — pause it indefinitely, or extend it until a specific date (e.g. an open dispute,
a support escalation, a data-export in flight).

**Key realization — the mechanism already exists.** Migration `017_tenants_legal_hold` added
`hold_until TIMESTAMPTZ` + `freeze_reason TEXT`, and the sweeper's deadline query already
skips any held tenant (`services/iam/db/queries.sql:110-117`):

```sql
AND (freeze_reason IS NULL
     OR (hold_until IS NOT NULL AND hold_until <= $1::timestamptz))
```

So a hold = "sweeper skips this tenant until `hold_until` (or forever if NULL)". That is
exactly pause/extend. **No new column, no new migration, no sweeper/query change.**

**What's missing** is only the *write path* to apply a hold to an already-suspended tenant:

- `SuspendTenant` (`handler.go:387`) accepts hold params, but its repo call
  (`UpdateTenantStatus`) also re-writes `status='suspended'` — so using it to hold a tenant
  already in `grace`/`deactivated` would **regress** its status. Unsafe for this purpose.
- `ClearTenantLegalHold` (`handler.go:414`) only *clears* — it is the resume/un-pause half,
  already shipped. #119 adds the *apply* half.

**Scope decisions (locked in brainstorming):**
- **Extend semantics = hold-until date** (reuse legal-hold), not anchor re-stamp, not a new
  override column. Freezes at the current stage until `hold_until`, then natural progression
  resumes. No migration.
- **Deliver hold/extend only.** Force-advance (skip a tenant forward a stage) is deferred to a
  separate ticket. Resume/un-pause is already `ClearTenantLegalHold`.
- **Single hold slot:** `freeze_reason`/`hold_until` is one pair per tenant. A retention
  extension and a genuine legal hold share it. Applying a hold when one already exists is
  **rejected** unless the caller passes `overwrite=true` (the error surfaces the current
  `freeze_reason` so the operator sees what they'd replace).

## Component 1 — proto

`proto/thittam/iam/v1/iam.proto`, in the `// --- Tenants ---` block (after
`ClearTenantLegalHold`, `iam.proto:52-61`):

```proto
rpc SetTenantRetention(SetTenantRetentionRequest) returns (Tenant);
```

New request message (near `ClearTenantLegalHoldRequest`, `iam.proto:331`):

```proto
message SetTenantRetentionRequest {
  string id = 1;
  // Required, non-empty. Written to freeze_reason; presence freezes the sweeper.
  string freeze_reason = 2;
  // Unset = indefinite pause. Set = extend the hold until this time; must be in the future.
  optional google.protobuf.Timestamp hold_until = 3;
  // Replace an existing active hold. Default false → the call is rejected if a hold exists.
  bool overwrite = 4;
}
```

gRPC-only — no `google.api.http` annotation, matching the sibling tenant-admin RPCs. Returns
the full `Tenant` message like its siblings. Regenerate with `buf generate` (updates
`gen/iam/v1/`). This is an **additive** proto change (new RPC + new message) — passes
`buf breaking`.

## Component 2 — handler

`services/iam/handler.go`, mirroring `SuspendTenant` (`handler.go:387-412`):

```go
func (h *Handler) SetTenantRetention(ctx context.Context, req *iamv1.SetTenantRetentionRequest) (*iamv1.Tenant, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	if strings.TrimSpace(req.GetFreezeReason()) == "" {
		return nil, status.Error(codes.InvalidArgument, "freeze_reason is required")
	}
	var holdUntil *time.Time
	if t := req.HoldUntil; t != nil { // raw field: preserve proto3 presence
		v := t.AsTime()
		holdUntil = &v
	}
	tenant, err := h.svc.SetTenantRetention(ctx, id, holdUntil, req.GetFreezeReason(), req.GetOverwrite())
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(tenant), nil
}
```

## Component 3 — service

New method `Service.SetTenantRetention` (new file `services/iam/retention_override.go`, to keep
`lifecycle.go` focused on the sweeper state machine):

1. `tenant, err := s.repo.GetTenant(ctx, id)` — not found → `ErrTenantNotFound`.
2. **Status guard:** `tenant.Status` ∈ {`suspended`, `grace`, `deactivated`} (clock running).
   `active` (no clock to hold) and `purge_eligible` (terminal dead-end) → new sentinel
   `ErrTenantNotHoldable` → `FailedPrecondition`.
3. **hold_until guard:** compute `now := time.Now().UTC()`; if
   `holdUntil != nil && !holdUntil.After(now)` → `InvalidArgument` ("hold_until must be in the
   future"). No clock abstraction needed — tests use dates far in the past/future relative to
   real `now` (matching how the sibling service methods handle time).
4. **Collision guard:** if `tenant.FreezeReason != nil && *tenant.FreezeReason != "" && !overwrite`
   → new sentinel `ErrTenantHoldExists` → `FailedPrecondition`, message includes the current
   `freeze_reason`.
5. `updated, err := s.repo.SetTenantLegalHold(ctx, id, holdUntil, freezeReason)`.
6. Audit: `s.audit.Log(ctx, audit.ActionLegalHoldApplied, ...)` with metadata
   `{freeze_reason, hold_until, overwrote_previous: bool, previous_reason: *string}`; actor
   resolved from ctx (the platform-admin caller), mirroring how `ClearTenantLegalHold`'s
   service path logs.
7. Return `updated`.

Guards live in Go (not SQL) because the collision error must echo the existing `freeze_reason`,
which requires the read anyway; once read, both guards in Go yield precise `FailedPrecondition`
messages that a single guarded `UPDATE`'s 0-row result could not distinguish. The daily-cron
sweeper cadence makes the read-then-write race negligible.

## Component 4 — repository

Interface (`services/iam/repository.go`, `// Tenants` section) — new method, the mirror of
`ClearTenantLegalHold`:

```go
SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error)
```

sqlc query (`services/iam/db/queries.sql`, near `ClearTenantLegalHold` at `:64-79`):

```sql
-- name: SetTenantLegalHold :one
UPDATE tenants
   SET hold_until    = $2,
       freeze_reason = $3
 WHERE id = $1
RETURNING *;
```

`status`, `suspended_at`, `deactivated_at` are untouched — the whole point (no status
regression). Implementation in `services/iam/db/postgres.go` mirrors `ClearTenantLegalHold`
(`postgres.go:465-474`): map `pgx.ErrNoRows` → `iam.ErrTenantNotFound`, wrap params via
`pgTimestamptzFromTimePtr`/plain text. Run `sqlc generate`.

## Component 5 — error mapping

`services/iam/errors.go`: add sentinels `ErrTenantNotHoldable` and `ErrTenantHoldExists`; map
both to `codes.FailedPrecondition` in `grpcError`. Existing `ErrTenantNotFound` → `NotFound`
path is reused.

## Error handling summary

| Condition | Code |
|---|---|
| bad UUID / empty freeze_reason / past hold_until | `InvalidArgument` |
| tenant missing | `NotFound` |
| status active or purge_eligible | `FailedPrecondition` (`ErrTenantNotHoldable`) |
| hold already exists and `!overwrite` | `FailedPrecondition` (`ErrTenantHoldExists`, echoes reason) |
| not platform-admin | `PermissionDenied` (from `RequireRole`) |

## Testing (iam ≥ 85%)

- **Service unit** (`retention_override_test.go`): eligible-status matrix (suspended/grace/
  deactivated pass; active/purge_eligible rejected); past `hold_until` rejected; indefinite
  (nil) vs dated hold both write correctly; collision → reject without `overwrite`, succeed with
  `overwrite` and record `overwrote_previous`/`previous_reason` in audit; status/`suspended_at`
  unchanged after a hold (no regression); audit event emitted with expected metadata (fake audit
  logger).
- **Handler** (`handler_test.go`): `RequireRole` gate (non-admin → `PermissionDenied`); empty
  `freeze_reason` → `InvalidArgument`; `hold_until` presence round-trips to the service.
- **Repo integration** (real Postgres, `-tags=integration`): `SetTenantLegalHold` writes both
  columns and leaves status/timestamps intact; unknown id → `ErrTenantNotFound`.

## Non-goals

- Force-advance / skip-a-stage (deferred ticket).
- Surfacing `hold_until`/`freeze_reason` on the `Tenant` proto (still DB/domain-only).
- Any change to the sweeper, its query, or migrations.
- Proportional shifting of downstream deadlines (explicitly rejected in favor of hold-until).

## Files touched

`proto/thittam/iam/v1/iam.proto` (+`buf generate` → `gen/iam/v1/`), `services/iam/handler.go`,
`services/iam/retention_override.go` (new), `services/iam/repository.go`,
`services/iam/db/queries.sql` (+`sqlc generate` → `queries.sql.go`),
`services/iam/db/postgres.go`, `services/iam/errors.go`, plus the three test files.
