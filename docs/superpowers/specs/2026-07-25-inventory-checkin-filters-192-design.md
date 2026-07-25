# Inventory — CheckInAsset contract + list filters (#192) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-25
**Issue:** #192 (inventory: fix CheckInAsset contract + ListAssets category/search + ListCheckouts pagination) — #60 Phase C follow-up
**Branch:** `feat/inventory-checkin-filters-192` off `main` (`f52d249`)
**Migration:** one (inventory `002`)

## Goal

Make the inventory-management RPCs match what the UI already sends: `CheckInAsset`
resolves the open checkout from `asset_id` server-side (the UI never has a
`checkout_id`) and records damage; `ListAssets` gains `category_id`/`search`
filters; `ListCheckouts` gains `limit`/`after` pagination. Then annotate the
changed RPCs onto the existing inventory `:9084` C2 gateway (already wired).

## Context

The inventory gateway (`:9084`, #60 C2) routes `/api/v1/assets` to
inventory-management. The UI (`web/src/lib/api/inventory.ts`) is ahead of the
backend:

- `checkInAsset(assetId, data)` calls `POST …/assets/{assetId}/check-in` with
  `{condition_in, notes?, report_damage?, damage_severity?, damage_description?,
  repair_cost?}` — it **never sends `checkout_id`**, but the current
  `CheckInAssetRequest` requires it and the handler `uuid.Parse`s it.
- `listAssets(params)` forwards `category_id`/`search`; the proto
  `ListAssetsRequest` has neither.
- `listCheckouts(assetId, params)` forwards `limit`/`after`; the proto
  `ListCheckoutsRequest` has only `asset_id`.

Grounding map (main, inventory untouched by #191):

| Piece | CheckInAsset | ListAssets category/search | ListCheckouts limit/after |
|---|---|---|---|
| DB | `asset_checkouts` has **no damage columns** → migration | `assets.category_id` indexed; name/asset_code searchable — no migration | no schema change |
| SQL | `GetActiveCheckout` (open checkout by asset_id) **exists but unwired**; `CheckinAsset` sets only `condition_in` → extend + migration | `ListAssets` SQL already filters `category_id` (`$3`, hardcoded `""`); **no search clause** → new ILIKE | `ListCheckouts` supports `LIMIT/OFFSET`; no cursor → add keyset |
| repo/svc/handler | resolve-from-asset + damage params must be built | thread `categoryID`/`search` | thread `limit`/`after` |
| proto | keep `checkout_id` (deprecate), add damage fields to request + `AssetCheckout` | add `category_id`,`search` | add `limit`,`after` |

Permission strings in use: `inventory:checkout`, `inventory:read`,
`inventory:write`. Asset `status` CHECK already includes `available` +
`under_repair` (no migration for the status transition).

## Design

### 1. Migration `inventory/002` — damage columns

`migrations/inventory/002_add_checkout_damage.up.sql`:
```sql
ALTER TABLE asset_checkouts
    ADD COLUMN notes              TEXT,
    ADD COLUMN report_damage      BOOLEAN       NOT NULL DEFAULT false,
    ADD COLUMN damage_severity    TEXT,
    ADD COLUMN damage_description TEXT,
    ADD COLUMN repair_cost        NUMERIC(14,2);
```
Down drops all five in reverse. `repair_cost` is money → `NUMERIC(14,2)` /
`decimal.Decimal`, never float.

**⚠️ inventory has NO `001_*.down.sql`** (only the `.up.sql` exists) — `002`
is the first down-migration for this service. Main is currently green with the
up-only `001`, so CI's `Migration Validate (up + down)` tolerates a missing
down (or doesn't run inventory down-to-zero). The plan must **verify** how that
job treats inventory and, if it requires a full down chain, add `001`'s down as
part of this work; otherwise ship only `002.{up,down}` (retro-fixing `001` is
out of #192's scope unless CI demands it).

### 2. Proto (`inventory.proto`)

- **`CheckInAssetRequest`** — KEEP `checkout_id` (removing a field is a buf
  `FILE` breaking change) but mark it `// Deprecated: ignored; the open checkout
  is resolved from asset_id server-side.` Keep `asset_id`, `condition_in`. ADD:
  `string notes`, `bool report_damage`, `string damage_severity`, `string
  damage_description`, `string repair_cost` (new field numbers, appended).
- **`AssetCheckout`** message — add the same five damage fields so the response
  reflects what was recorded (`repair_cost` as a decimal string).
- **`ListAssetsRequest`** — add `string category_id`, `string search`.
- **`ListCheckoutsRequest`** — add `int32 limit`, `string after`.
- `CheckInAsset` keeps its existing `POST …/assets/{asset_id}/check-in`
  annotation (already present from C2); no new routes, so **no Kong change**.
  Adding fields/annotations is not buf-breaking. `buf generate proto`, scope the
  commit to `gen/inventory/v1/` (revert cross-service drift, per C2).

### 3. Service (`services/inventory/service.go`)

- **`CheckInAsset`** — change the signature to resolve the checkout from the
  asset. Proposed:
  ```go
  type CheckInInput struct {
      ConditionIn       string
      Notes             string
      ReportDamage      bool
      DamageSeverity    string
      DamageDescription string
      RepairCost        *decimal.Decimal // nil when omitted
  }
  func (s *Service) CheckInAsset(ctx, tenantID, assetID uuid.UUID, in CheckInInput) error
  ```
  Flow: `co, err := s.repo.GetActiveCheckout(ctx, tenantID, assetID)` → if none,
  return `ErrNoActiveCheckout` (→ `FailedPrecondition`); else
  `s.repo.CheckInAsset(ctx, tenantID, co.ID, in)` (writes `condition_in` + the
  damage columns); then set asset status: **`under_repair` if
  `in.ReportDamage` else `available`** via the existing `UpdateAssetStatus`.
- **`ListAssets`** — add `categoryID, search string` params, threaded to the repo.
- **`ListCheckouts`** — add `limit int, after string` params, threaded to the repo.

### 4. Repo (`services/inventory/`)

- Add **`GetActiveCheckout(ctx, tenantID, assetID uuid.UUID) (*AssetCheckout,
  error)`** to the `Repository` interface + Postgres impl, wiring the existing
  generated `GetActiveCheckout` query (map `pgx.ErrNoRows` →
  `ErrNoActiveCheckout`).
- **`CheckInAsset`** repo method + the `CheckinAsset` SQL grow to set the five
  damage columns (`notes/report_damage/damage_severity/damage_description/
  repair_cost`) alongside `checked_in_at`/`condition_in`.
- **`ListAssets`** — interface/impl gain `categoryID` (stop hardcoding
  `Column3: ""`) + `search`; the SQL gains a search clause
  `AND ($N = '' OR name ILIKE '%'||$N||'%' OR asset_code ILIKE '%'||$N||'%')`.
- **`ListCheckouts`** — interface/impl gain `limit`/`after`; `after` is an
  **opaque keyset cursor** = the `checked_out_at` RFC3339 of the last row
  (`ORDER BY checked_out_at DESC`, `AND ($after = '' OR checked_out_at < $after)`),
  avoiding OFFSET drift. `limit` capped (reuse the existing 200 ceiling as the max).
- `AssetCheckout` domain struct gains the five damage fields (with
  `RepairCost *decimal.Decimal`).

### 5. Handlers (`services/inventory/handler.go`)

Gate order tenant (`Unauthenticated`) → `RequirePermission` → parse → service,
matching the existing handlers.
- **`CheckInAsset`** → perm `inventory:checkout` (a check-in/out operation;
  confirm against the existing `CheckOutAsset`/`CheckInAsset` gate in the plan).
  Parse `asset_id` (required, `InvalidArgument` on bad UUID); **ignore
  `checkout_id`**. Build `CheckInInput`: `repair_cost` via `decimal.NewFromString`
  only when non-empty (→ `InvalidArgument` on garbage, and reject a negative
  cost); when `report_damage` is true, require a non-empty `damage_severity`
  (→ `InvalidArgument`). Re-fetch and map to proto.
- **`ListAssets`** → perm `inventory:read`; pass `req.GetCategoryId()`,
  `req.GetSearch()` through.
- **`ListCheckouts`** → perm `inventory:read`; pass `req.GetLimit()`,
  `req.GetAfter()` through.

## Testing

- **Service tests:** `CheckInAsset` resolves the open checkout (mock
  `GetActiveCheckout`), sets `under_repair` when `ReportDamage` and `available`
  otherwise, and returns `ErrNoActiveCheckout` when there's no open checkout;
  `ListAssets`/`ListCheckouts` thread the new params (recording mock).
- **Handler tests** per changed RPC: `CheckInAsset` Success (damage +
  non-damage), Denied, NoTenant, InvalidID (bad `asset_id`), plus bad/negative
  `repair_cost` → `InvalidArgument` and `report_damage` without severity →
  `InvalidArgument`; `ListAssets`/`ListCheckouts` Success asserting the new
  filter/pagination args reach the service, plus Denied/NoTenant.
- `buf generate proto` clean; `go build ./...` + `go vet ./...` (whole tree —
  the `Repository`/`Service` signature changes ripple to every inventory test
  double: `mockRepo` in `service_test.go`, plus any e2e/integration doubles —
  update all so the tree compiles); `go test ./services/inventory/... -race`;
  keeps inventory ≥ 75% floor.
- `sqlc generate` fresh (Codegen Freshness gate); scope to
  `services/inventory/db/`.

## Non-goals

- **Not fixing ListAssets pagination** — its `after` is already dead (handler
  passes offset `0`); #192 only adds `category_id`/`search` there. Note the dead
  `after` as a deferred observation, don't fix it here.
- No full-text/trigram index for search — plain `ILIKE` (a perf index is a later
  follow-up).
- `checkout_id` stays in the proto (deprecated, ignored) — never removed.
- Not #191 (expense) — already merged.

## Review weight

Touches `inventory` + a migration + money (`repair_cost`) + proto/generated code
→ standard 2 approvals. Whole-branch review on the most capable model.
