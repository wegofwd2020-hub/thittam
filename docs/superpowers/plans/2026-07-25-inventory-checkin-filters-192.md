# Inventory — CheckInAsset contract + list filters (#192) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make CheckInAsset resolve the open checkout from `asset_id` server-side and record damage, add ListAssets `category_id`/`search` filters and ListCheckouts `limit`/`after` pagination, and annotate onto the existing inventory `:9084` gateway.

**Architecture:** One migration adds five damage columns to `asset_checkouts`. A single data-layer task changes the four affected `Repository` methods + wires the already-generated `GetActiveCheckout`; then proto, then the service/handler logic split by feature. Interface signature changes ripple to both test doubles — a whole-tree `go vet` gate is mandatory.

**Tech Stack:** Go 1.25, protobuf + buf, sqlc (inventory has a `numeric→decimal.Decimal` override), Postgres, `pkg/server` gateway (inventory already wired at :9084).

## Global Constraints

- **Money is `decimal.Decimal` / `NUMERIC(14,2)`, never `float64`** — `repair_cost` is money.
- All SQL parameterized (sqlc); every query tenant-scoped (`WHERE ... AND tenant_id = ...`).
- **Widening/changing the `Repository` interface requires whole-tree satisfaction:** after any interface change, `go vet ./...` (NOT just `services/inventory`) must be clean. The two doubles that implement `inventory.Repository`: `mockRepo` (`services/inventory/service_test.go`) and `inventoryMock` (`tests/integration/vertical/mocks_test.go`). Update BOTH; let `go vet ./...` catch any third.
- **sqlc + buf generated code committed fresh** (CI gates on `sqlc generate` / `buf generate` producing no diff). sqlc scope: `services/inventory/db/`. buf scope: `gen/inventory/v1/` (revert cross-service drift).
- **buf `FILE` breaking category:** never remove the existing `checkout_id` proto field — deprecate by comment, ignore it.
- Handler gate order: tenant (`Unauthenticated`) → `RequirePermission` → parse → service. Perms: CheckInAsset → `inventory:checkout`; ListAssets/ListCheckouts → `inventory:read`.
- **No Kong change** — CheckInAsset's `POST /api/v1/assets/{asset_id}/check-in` route already exists; only fields change.
- LOCAL DB SAFETY: never `docker compose -v`/`down`/`up` on `infra/local/`; `go build`/`sqlc generate`/`go test` are static/DSN-gated.
- Commits Conventional-Commits, scope `inventory`, ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `migrations/inventory/002_add_checkout_damage.{up,down}.sql` | 5 damage columns | 1 |
| `services/inventory/db/queries.sql` + regenerated `.go` | CheckinAsset damage, ListAssets search, ListCheckouts cursor | 1 |
| `services/inventory/models.go` | `CheckInInput` type + `AssetCheckout` damage fields | 1 |
| `services/inventory/errors.go` | `ErrNoActiveCheckout` | 1 |
| `services/inventory/repository.go` + `db/postgres.go` | 4 method sig changes + `GetActiveCheckout` | 1 |
| `services/inventory/service_test.go` (`mockRepo`) + `tests/integration/vertical/mocks_test.go` (`inventoryMock`) | double updates | 1 |
| `proto/thittam/inventory/v1/inventory.proto` + `gen/inventory/v1/*` | request/response field additions | 2 |
| `services/inventory/service.go` + `handler.go` + `handler_test.go` | CheckInAsset logic + converter + tests | 3 |
| `services/inventory/service.go` + `handler.go` + `*_test.go` | ListAssets/ListCheckouts logic + tests | 4 |

---

### Task 1: Data layer — migration, SQL, repository, doubles

**Files:**
- Create: `migrations/inventory/002_add_checkout_damage.{up,down}.sql`
- Modify: `services/inventory/db/queries.sql` (+ regenerated `queries.sql.go`), `models.go`, `errors.go`, `repository.go`, `db/postgres.go`, `service_test.go`, `tests/integration/vertical/mocks_test.go`

**Interfaces:**
- Produces (consumed by Tasks 3/4):
  - `CheckInInput` struct (below).
  - `Repository.GetActiveCheckout(ctx context.Context, tenantID, assetID uuid.UUID) (*AssetCheckout, error)`.
  - `Repository.CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, in CheckInInput) (*AssetCheckout, error)` (was `(…, conditionIn string) error`).
  - `Repository.ListAssets(ctx context.Context, tenantID uuid.UUID, status, categoryID, search string, limit, offset int) ([]Asset, error)` (added `categoryID, search`).
  - `Repository.ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID, limit int, after string) ([]AssetCheckout, error)` (added `limit, after`).
  - `AssetCheckout` gains `Notes string`, `ReportDamage bool`, `DamageSeverity string`, `DamageDescription string`, `RepairCost *decimal.Decimal`.
  - `ErrNoActiveCheckout`.

- [ ] **Step 1: Migration**

`002_add_checkout_damage.up.sql`:
```sql
-- CheckInAsset (#192) records damage reported at check-in.
ALTER TABLE asset_checkouts
    ADD COLUMN notes              TEXT,
    ADD COLUMN report_damage      BOOLEAN       NOT NULL DEFAULT false,
    ADD COLUMN damage_severity    TEXT,
    ADD COLUMN damage_description TEXT,
    ADD COLUMN repair_cost        NUMERIC(14,2);
```
`002_add_checkout_damage.down.sql`:
```sql
ALTER TABLE asset_checkouts
    DROP COLUMN IF EXISTS repair_cost,
    DROP COLUMN IF EXISTS damage_description,
    DROP COLUMN IF EXISTS damage_severity,
    DROP COLUMN IF EXISTS report_damage,
    DROP COLUMN IF EXISTS notes;
```
**Note:** `migrations/inventory/` currently has only `001_*.up.sql` (no down). This `002.down` is the service's first. Before committing, check whether CI's `Migration Validate (up + down)` requires a full down chain: run the migrations locally against a THROWAWAY uniquely-named container OR rely on CI as the gate (do NOT touch `infra/local/`). If the plan's implementer cannot verify, note it in the report — the reviewer/CI decides. Do NOT retroactively add `001.down` unless CI fails without it (out of #192 scope).

- [ ] **Step 2: SQL — CheckinAsset (damage), ListAssets (search), ListCheckouts (cursor)**

In `services/inventory/db/queries.sql`:

`CheckinAsset` — set the damage columns:
```sql
-- name: CheckinAsset :one
UPDATE asset_checkouts
SET checked_in_at = now(),
    condition_in = $3,
    notes = $4,
    report_damage = $5,
    damage_severity = $6,
    damage_description = $7,
    repair_cost = $8
WHERE id = $1 AND tenant_id = $2
RETURNING *;
```
`ListAssets` — add a search clause (`$6`) over name + asset_code:
```sql
-- name: ListAssets :many
SELECT * FROM assets
WHERE tenant_id = $1
  AND ($2 = '' OR status = $2)
  AND ($3 = '' OR category_id = $3)
  AND ($6 = '' OR name ILIKE '%' || $6 || '%' OR asset_code ILIKE '%' || $6 || '%')
ORDER BY name ASC
LIMIT $4 OFFSET $5;
```
`ListCheckouts` — replace OFFSET paging with a keyset `after` cursor on `checked_out_at`; drop the unused `production_id` narg:
```sql
-- name: ListCheckouts :many
SELECT * FROM asset_checkouts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (sqlc.narg('asset_id')::uuid IS NULL OR asset_id = sqlc.narg('asset_id')::uuid)
  AND (sqlc.narg('after')::timestamptz IS NULL OR checked_out_at < sqlc.narg('after')::timestamptz)
ORDER BY checked_out_at DESC
LIMIT sqlc.arg('limit')::int;
```

- [ ] **Step 3: Regenerate sqlc + confirm scope**

```bash
sqlc generate
git status --porcelain services/ | grep -v 'services/inventory/db/' || echo "SCOPED OK"
```
Only `services/inventory/db/` should change. If another service's db gen changed (e.g. reporting shares a schema source), that's expected drift ONLY if that service reads `asset_checkouts`/`assets` — otherwise revert it: `git checkout -- <path>`. Confirm the new generated types:
- `CheckinAssetParams` gains `Notes/ReportDamage/DamageSeverity/DamageDescription/RepairCost` — note their EXACT generated Go types (text → `pgtype.Text`; bool → `bool`; numeric → the inventory override type, likely `pgtype.Numeric` or `decimal.Decimal` — match whatever `purchase_cost` uses on the generated `Asset` row).
- `ListAssetsParams` gains a field for `$6` (search).
- `ListCheckoutsParams` now has `TenantID`, `AssetID` (nullable), `After` (nullable timestamptz), `Limit`.
- `GetActiveCheckout` is already generated (`GetActiveCheckoutParams{AssetID, TenantID}`).

- [ ] **Step 4: Domain types — `CheckInInput` + `AssetCheckout` fields**

In `services/inventory/models.go` (imports `decimal` already), add to the `AssetCheckout` struct after `ConditionIn`:
```go
	Notes             string           `json:"notes,omitempty"`
	ReportDamage      bool             `json:"report_damage"`
	DamageSeverity    string           `json:"damage_severity,omitempty"`
	DamageDescription string           `json:"damage_description,omitempty"`
	RepairCost        *decimal.Decimal `json:"repair_cost,omitempty"`
```
And add the check-in input type:
```go
// CheckInInput carries the check-in payload; the open checkout is resolved from
// the asset server-side, so no checkout ID is required.
type CheckInInput struct {
	ConditionIn       string
	Notes             string
	ReportDamage      bool
	DamageSeverity    string
	DamageDescription string
	RepairCost        *decimal.Decimal // nil when the client omits it
}
```

- [ ] **Step 5: `ErrNoActiveCheckout`**

In `services/inventory/errors.go`, add to the `var (...)` block:
```go
	ErrNoActiveCheckout = errors.New("inventory: no open checkout for this asset")
```

- [ ] **Step 6: Repository interface**

In `services/inventory/repository.go`, change the `// Checkouts` block to:
```go
	// Checkouts
	CheckOutAsset(ctx context.Context, c *AssetCheckout) error
	GetActiveCheckout(ctx context.Context, tenantID, assetID uuid.UUID) (*AssetCheckout, error)
	CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, in CheckInInput) (*AssetCheckout, error)
	GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*AssetCheckout, error)
	ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID, limit int, after string) ([]AssetCheckout, error)
```
And change `ListAssets` in the `// Assets` block to:
```go
	ListAssets(ctx context.Context, tenantID uuid.UUID, status, categoryID, search string, limit, offset int) ([]Asset, error)
```

- [ ] **Step 7: Postgres impls (`db/postgres.go`)**

Implement/adjust (match generated field names/types; `pgtype`, `fmt`, `time` importing as needed — add `"time"` if not present):
```go
func (p *Postgres) GetActiveCheckout(ctx context.Context, tenantID, assetID uuid.UUID) (*inventory.AssetCheckout, error) {
	row, err := p.q.GetActiveCheckout(ctx, GetActiveCheckoutParams{
		AssetID:  pgtype.UUID{Bytes: assetID, Valid: true},
		TenantID: tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, inventory.ErrNoActiveCheckout
	}
	if err != nil {
		return nil, fmt.Errorf("inventory/db: get active checkout: %w", err)
	}
	co := checkoutFromRow(row) // reuse the existing row→domain mapper the other checkout methods use
	return &co, nil
}

func (p *Postgres) CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, in inventory.CheckInInput) (*inventory.AssetCheckout, error) {
	row, err := p.q.CheckinAsset(ctx, CheckinAssetParams{
		ID:                checkoutID,
		TenantID:          tenantID,
		ConditionIn:       pgTextFromString(in.ConditionIn),
		Notes:             pgTextFromString(in.Notes),
		ReportDamage:      in.ReportDamage,
		DamageSeverity:    pgTextFromString(in.DamageSeverity),
		DamageDescription: pgTextFromString(in.DamageDescription),
		RepairCost:        pgNumericFromDecimalPtr(in.RepairCost), // see note
	})
	if err != nil {
		return nil, fmt.Errorf("inventory/db: check in asset: %w", err)
	}
	co := checkoutFromRow(row)
	return &co, nil
}
```
Notes for the implementer:
- Use the EXISTING helpers this file already has (there is a `pgTextFromString` per the codebase; find the existing row→`AssetCheckout` mapper the current `CheckInAsset`/`GetCheckout` use and extend it to copy the five new columns, incl. `RepairCost` as `*decimal.Decimal` — nil when the DB value is NULL). If `repair_cost`'s generated type is `decimal.Decimal` (via the numeric override) rather than `pgtype.Numeric`, adapt: pass `decimal.Decimal` directly and treat nil `in.RepairCost` as `decimal.Zero` OR use the nullable generated type — match what compiles against the generated `CheckinAssetParams`.
- `ListAssets` impl: pass `Column6`/the generated search field = `search` and stop hardcoding the category to `""` — pass `categoryID`.
- `ListCheckouts` impl:
```go
func (p *Postgres) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID, limit int, after string) ([]inventory.AssetCheckout, error) {
	var afterTS pgtype.Timestamptz
	if after != "" {
		t, err := time.Parse(time.RFC3339, after)
		if err != nil {
			return nil, fmt.Errorf("inventory/db: invalid after cursor: %w", err)
		}
		afterTS = pgtype.Timestamptz{Time: t, Valid: true}
	}
	rows, err := p.q.ListCheckouts(ctx, ListCheckoutsParams{
		TenantID: tenantID,
		AssetID:  pgtype.UUID{Bytes: assetID, Valid: true},
		After:    afterTS,
		Limit:    int32(limit),
	})
	// ... existing row→domain loop, now also copying the damage columns
}
```

- [ ] **Step 8: Update both test doubles**

`mockRepo` (`services/inventory/service_test.go`) — change the affected fn-field signatures + methods, and ADD a `getActiveCheckoutFn`:
```go
	getActiveCheckoutFn func(ctx context.Context, tenantID, assetID uuid.UUID) (*AssetCheckout, error)
	checkInAssetFn      func(ctx context.Context, tenantID, checkoutID uuid.UUID, in CheckInInput) (*AssetCheckout, error)
	listAssetsFn        func(ctx context.Context, tenantID uuid.UUID, status, categoryID, search string, limit, offset int) ([]Asset, error)
	listCheckoutsFn     func(ctx context.Context, tenantID, assetID uuid.UUID, limit int, after string) ([]AssetCheckout, error)
```
Update the four corresponding methods to the new signatures (default returns: `getActiveCheckoutFn`→`(&AssetCheckout{ID: uuid.New(), TenantID: tenantID, AssetID: assetID}, nil)`, `checkInAssetFn`→`(&AssetCheckout{ID: checkoutID, TenantID: tenantID}, nil)`, list defaults `nil, nil`) and add a `GetActiveCheckout` method. **Existing tests that call the old signatures will break — fix their call sites minimally** (e.g. an existing CheckInAsset test now sets `checkInAssetFn` with the new signature; a following task rewrites the handler-level CheckInAsset tests, so here only make the package COMPILE + keep existing non-checkin tests green).

`inventoryMock` (`tests/integration/vertical/mocks_test.go`) — update the three stubs + add one:
```go
func (m *inventoryMock) GetActiveCheckout(ctx context.Context, tid, assetID uuid.UUID) (*inventory.AssetCheckout, error) { return &inventory.AssetCheckout{}, nil }
func (m *inventoryMock) CheckInAsset(ctx context.Context, tid, checkoutID uuid.UUID, in inventory.CheckInInput) (*inventory.AssetCheckout, error) { return &inventory.AssetCheckout{}, nil }
func (m *inventoryMock) ListAssets(ctx context.Context, tid uuid.UUID, status, categoryID, search string, limit, offset int) ([]inventory.Asset, error) { return nil, nil }
func (m *inventoryMock) ListCheckouts(ctx context.Context, tid, assetID uuid.UUID, limit int, after string) ([]inventory.AssetCheckout, error) { return nil, nil }
```

- [ ] **Step 9: Gate — whole tree**

```bash
go build ./... && go vet ./...
```
Expected: exit 0. `go vet ./...` (whole tree) is REQUIRED — it is the only check that catches an un-updated double. If it names any implementer besides the two above, update that one too.

- [ ] **Step 10: Commit**

```bash
git add migrations/inventory services/inventory/db services/inventory/models.go services/inventory/errors.go services/inventory/repository.go services/inventory/service_test.go tests/integration/vertical/mocks_test.go
git commit -m "feat(inventory): data layer for check-in damage + list filters — migration, SQL, repo (#192)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```
(Note: `service.go`/`handler.go` still reference the OLD service-internal calls and will not compile against the new repo signatures — Tasks 3/4 fix the service/handler. If `go build ./...` fails at Step 9 because `service.go` calls the old `repo.CheckInAsset`/`ListAssets`/`ListCheckouts` signatures, make the MINIMAL service.go edits needed to compile — update `Service.CheckInAsset`/`ListAssets`/`ListCheckouts` call sites to the new repo signatures with straight pass-through — and include `service.go` in this commit. The full service logic/tests land in Tasks 3/4; here just keep the tree building.)

---

### Task 2: Proto — request/response fields + gen

**Files:**
- Modify: `proto/thittam/inventory/v1/inventory.proto` (+ regenerated `gen/inventory/v1/*`)

**Interfaces:**
- Produces: `CheckInAssetRequest` gains `notes/report_damage/damage_severity/damage_description/repair_cost`; `AssetCheckout` gains the same five; `ListAssetsRequest` gains `category_id/search`; `ListCheckoutsRequest` gains `limit/after`.

- [ ] **Step 1: Edit the messages**

In `inventory.proto`:
```proto
message CheckInAssetRequest {
  string checkout_id = 1;  // Deprecated: ignored; the open checkout is resolved from asset_id server-side.
  string asset_id = 2;
  string condition_in = 3;
  string notes = 4;
  bool report_damage = 5;
  string damage_severity = 6;
  string damage_description = 7;
  string repair_cost = 8;
}
```
Add to the `AssetCheckout` message (after its existing fields, next free numbers):
```proto
  string notes = 12;
  bool report_damage = 13;
  string damage_severity = 14;
  string damage_description = 15;
  string repair_cost = 16;
```
(Use the ACTUAL next free field numbers in that message — read it first; the numbers above assume 11 existing fields.)
`ListAssetsRequest` — add after `after`:
```proto
  string category_id = 4;
  string search = 5;
```
`ListCheckoutsRequest` — add after `asset_id`:
```proto
  int32 limit = 2;
  string after = 3;
```
(Again, use each message's real next field numbers.)

- [ ] **Step 2: Generate + scope + verify**

```bash
buf generate proto
git checkout -- $(git status --porcelain gen/ | grep -v 'gen/inventory/' | awk '{print $2}')
go build ./...
```
Expected: `go build` exit 0 (handlers still map old fields until Tasks 3/4 — that compiles). Confirm no buf-breaking error (adding fields is safe).

- [ ] **Step 3: Commit**

```bash
git add proto/thittam/inventory/v1/inventory.proto gen/inventory/v1
git commit -m "feat(inventory): add check-in damage + list filter/pagination proto fields (#192)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: CheckInAsset — service + handler + converter + tests

**Files:**
- Modify: `services/inventory/service.go`, `handler.go`, `handler_test.go`, `service_test.go`

**Interfaces:**
- Consumes: Task 1's `CheckInInput`, `GetActiveCheckout`, `CheckInAsset(...) (*AssetCheckout, error)`, `ErrNoActiveCheckout`; Task 2's `CheckInAssetRequest` damage fields + `AssetCheckout` proto fields.
- Produces: `Service.CheckInAsset(ctx, tenantID, assetID uuid.UUID, in CheckInInput) (*AssetCheckout, error)`.

- [ ] **Step 1: Service method**

Replace `Service.CheckInAsset` in `service.go` (imports need `decimal`? no — the input carries it; keep imports as needed):
```go
func (s *Service) CheckInAsset(ctx context.Context, tenantID, assetID uuid.UUID, in CheckInInput) (*AssetCheckout, error) {
	co, err := s.repo.GetActiveCheckout(ctx, tenantID, assetID)
	if err != nil {
		return nil, err // ErrNoActiveCheckout maps to FailedPrecondition
	}
	updated, err := s.repo.CheckInAsset(ctx, tenantID, co.ID, in)
	if err != nil {
		return nil, err
	}
	status := "available"
	if in.ReportDamage {
		status = "under_repair"
	}
	if err := s.repo.UpdateAssetStatus(ctx, tenantID, assetID, status); err != nil {
		return nil, err
	}
	return updated, nil
}
```

- [ ] **Step 2: grpcErr mapping**

In `handler.go`'s `grpcErr`, add an arm:
```go
	case errors.Is(err, ErrNoActiveCheckout):
		return status.Error(codes.FailedPrecondition, err.Error())
```

- [ ] **Step 3: Extend `checkoutToProto` with damage fields**

In `handler.go`, add to `checkoutToProto` before the `return`:
```go
	out.Notes = c.Notes
	out.ReportDamage = c.ReportDamage
	out.DamageSeverity = c.DamageSeverity
	out.DamageDescription = c.DamageDescription
	if c.RepairCost != nil {
		out.RepairCost = c.RepairCost.StringFixed(2)
	}
```

- [ ] **Step 4: Rewrite the CheckInAsset handler**

Replace `Handler.CheckInAsset` in `handler.go` (imports already include `decimal`, `strings`? add `"strings"` if missing):
```go
func (h *Handler) CheckInAsset(ctx context.Context, req *inventoryv1.CheckInAssetRequest) (*inventoryv1.AssetCheckout, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "inventory:checkout"); err != nil {
		return nil, err
	}
	assetID, err := uuid.Parse(req.GetAssetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid asset_id")
	}
	in := CheckInInput{
		ConditionIn:       req.GetConditionIn(),
		Notes:             req.GetNotes(),
		ReportDamage:      req.GetReportDamage(),
		DamageSeverity:    req.GetDamageSeverity(),
		DamageDescription: req.GetDamageDescription(),
	}
	if s := req.GetRepairCost(); s != "" {
		cost, err := decimal.NewFromString(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid repair_cost: must be a decimal string")
		}
		if cost.IsNegative() {
			return nil, status.Error(codes.InvalidArgument, "repair_cost must not be negative")
		}
		in.RepairCost = &cost
	}
	if in.ReportDamage && strings.TrimSpace(in.DamageSeverity) == "" {
		return nil, status.Error(codes.InvalidArgument, "damage_severity is required when report_damage is set")
	}
	co, err := h.svc.CheckInAsset(ctx, tenantID, assetID, in)
	if err != nil {
		return nil, grpcErr(err)
	}
	return checkoutToProto(co), nil
}
```
(The `checkout_id` field is intentionally not read.)

- [ ] **Step 5: Tests**

In `service_test.go` add:
```go
func TestService_CheckInAsset_DamageSetsUnderRepair(t *testing.T) {
	var gotStatus string
	assetID := uuid.New()
	svc := NewService(&mockRepo{
		getActiveCheckoutFn: func(_ context.Context, tid, aid uuid.UUID) (*AssetCheckout, error) { return &AssetCheckout{ID: uuid.New(), TenantID: tid, AssetID: aid}, nil },
		checkInAssetFn:      func(_ context.Context, tid, cid uuid.UUID, in CheckInInput) (*AssetCheckout, error) { return &AssetCheckout{ID: cid, TenantID: tid, ReportDamage: in.ReportDamage}, nil },
		updateAssetStatusFn: func(_ context.Context, _, _ uuid.UUID, s string) error { gotStatus = s; return nil },
	})
	_, err := svc.CheckInAsset(context.Background(), uuid.New(), assetID, CheckInInput{ReportDamage: true, DamageSeverity: "severe"})
	require.NoError(t, err)
	assert.Equal(t, "under_repair", gotStatus)
}

func TestService_CheckInAsset_NoDamageSetsAvailable(t *testing.T) {
	var gotStatus string
	svc := NewService(&mockRepo{
		getActiveCheckoutFn: func(_ context.Context, tid, aid uuid.UUID) (*AssetCheckout, error) { return &AssetCheckout{ID: uuid.New(), TenantID: tid, AssetID: aid}, nil },
		checkInAssetFn:      func(_ context.Context, tid, cid uuid.UUID, in CheckInInput) (*AssetCheckout, error) { return &AssetCheckout{ID: cid, TenantID: tid}, nil },
		updateAssetStatusFn: func(_ context.Context, _, _ uuid.UUID, s string) error { gotStatus = s; return nil },
	})
	_, err := svc.CheckInAsset(context.Background(), uuid.New(), uuid.New(), CheckInInput{ConditionIn: "good"})
	require.NoError(t, err)
	assert.Equal(t, "available", gotStatus)
}

func TestService_CheckInAsset_NoOpenCheckout(t *testing.T) {
	svc := NewService(&mockRepo{
		getActiveCheckoutFn: func(_ context.Context, _, _ uuid.UUID) (*AssetCheckout, error) { return nil, ErrNoActiveCheckout },
	})
	_, err := svc.CheckInAsset(context.Background(), uuid.New(), uuid.New(), CheckInInput{})
	assert.ErrorIs(t, err, ErrNoActiveCheckout)
}
```
In `handler_test.go` add the handler trio + validation cases (use the existing `ctxWithTenant`/`ctxWithVertical`/`allowAllPerm`/`denyPerm`/`newHandler` helpers — confirm their exact names against the file; the CheckOutAsset tests are the template):
```go
func TestHandler_CheckInAsset_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getActiveCheckoutFn: func(_ context.Context, tid, aid uuid.UUID) (*AssetCheckout, error) { return &AssetCheckout{ID: uuid.New(), TenantID: tid, AssetID: aid}, nil },
		checkInAssetFn:      func(_ context.Context, tid, cid uuid.UUID, in CheckInInput) (*AssetCheckout, error) { return &AssetCheckout{ID: cid, TenantID: tid, ConditionIn: in.ConditionIn}, nil },
		updateAssetStatusFn: func(_ context.Context, _, _ uuid.UUID, _ string) error { return nil },
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.CheckInAsset(ctxWithTenant(tenantID), &inventoryv1.CheckInAssetRequest{AssetId: uuid.New().String(), ConditionIn: "good"})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_CheckInAsset_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getActiveCheckoutFn: func(_ context.Context, _, _ uuid.UUID) (*AssetCheckout, error) { t.Fatal("must not reach repo when denied"); return nil, nil },
	})).WithPermissionChecker(denyPerm{})
	_, err := h.CheckInAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckInAssetRequest{AssetId: uuid.New().String(), ConditionIn: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CheckInAsset_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckInAsset(ctxWithVertical(), &inventoryv1.CheckInAssetRequest{AssetId: uuid.New().String(), ConditionIn: "x"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CheckInAsset_InvalidAssetID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckInAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckInAssetRequest{AssetId: "bad", ConditionIn: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CheckInAsset_BadRepairCost(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckInAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckInAssetRequest{AssetId: uuid.New().String(), RepairCost: "abc"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CheckInAsset_DamageWithoutSeverity(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckInAsset(ctxWithTenant(uuid.New()), &inventoryv1.CheckInAssetRequest{AssetId: uuid.New().String(), ReportDamage: true})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
```

- [ ] **Step 6: Gate + commit**

```bash
go test ./services/inventory/... -race && go build ./... && go vet ./services/inventory/ && gofmt -l services/inventory/*.go
git add services/inventory/service.go services/inventory/handler.go services/inventory/handler_test.go services/inventory/service_test.go
git commit -m "feat(inventory): CheckInAsset resolves checkout from asset + records damage (#192)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: ListAssets + ListCheckouts — service + handlers + tests

**Files:**
- Modify: `services/inventory/service.go`, `handler.go`, `handler_test.go`, `service_test.go`

**Interfaces:**
- Consumes: Task 1's `ListAssets(...categoryID, search...)` / `ListCheckouts(...limit, after...)` repo signatures; Task 2's `ListAssetsRequest.category_id/search` + `ListCheckoutsRequest.limit/after`.

- [ ] **Step 1: Service methods**

Update in `service.go`:
```go
func (s *Service) ListAssets(ctx context.Context, tenantID uuid.UUID, status, categoryID, search string, limit, offset int) ([]Asset, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListAssets(ctx, tenantID, status, categoryID, search, limit, offset)
}

func (s *Service) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID, limit int, after string) ([]AssetCheckout, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListCheckouts(ctx, tenantID, assetID, limit, after)
}
```
(These replace the versions Task 1 stubbed to keep the build green; the old 200-slice cap in `ListCheckouts` is gone — the DB `LIMIT` enforces it now.)

- [ ] **Step 2: Handlers**

In `handler.go`, update `ListAssets` to pass the two new filters:
```go
	assets, err := h.svc.ListAssets(ctx, tenantID, req.GetStatus(), req.GetCategoryId(), req.GetSearch(), int(req.GetLimit()), 0)
```
And `ListCheckouts` to validate `after` + pass pagination (add `"time"` to imports if needed):
```go
	if after := req.GetAfter(); after != "" {
		if _, err := time.Parse(time.RFC3339, after); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid after: must be an RFC3339 timestamp")
		}
	}
	checkouts, err := h.svc.ListCheckouts(ctx, tenantID, assetID, int(req.GetLimit()), req.GetAfter())
```

- [ ] **Step 3: Tests**

In `service_test.go` add recording-mock tests that the new args reach the repo:
```go
func TestService_ListAssets_ThreadsCategoryAndSearch(t *testing.T) {
	var gotCat, gotSearch string
	svc := NewService(&mockRepo{
		listAssetsFn: func(_ context.Context, _ uuid.UUID, _ , cat, search string, _, _ int) ([]Asset, error) {
			gotCat, gotSearch = cat, search
			return nil, nil
		},
	})
	_, err := svc.ListAssets(context.Background(), uuid.New(), "", "cam", "arri", 20, 0)
	require.NoError(t, err)
	assert.Equal(t, "cam", gotCat)
	assert.Equal(t, "arri", gotSearch)
}

func TestService_ListCheckouts_ThreadsLimitAndAfter(t *testing.T) {
	var gotLimit int
	var gotAfter string
	svc := NewService(&mockRepo{
		listCheckoutsFn: func(_ context.Context, _, _ uuid.UUID, limit int, after string) ([]AssetCheckout, error) {
			gotLimit, gotAfter = limit, after
			return nil, nil
		},
	})
	_, err := svc.ListCheckouts(context.Background(), uuid.New(), uuid.New(), 50, "2026-07-25T00:00:00Z")
	require.NoError(t, err)
	assert.Equal(t, 50, gotLimit)
	assert.Equal(t, "2026-07-25T00:00:00Z", gotAfter)
}
```
In `handler_test.go` add:
```go
func TestHandler_ListCheckouts_BadAfter(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListCheckouts(ctxWithTenant(uuid.New()), &inventoryv1.ListCheckoutsRequest{AssetId: uuid.New().String(), After: "nonsense"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListAssets_PassesFilters(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	var gotCat, gotSearch string
	h := NewHandler(NewService(&mockRepo{
		listAssetsFn: func(_ context.Context, _ uuid.UUID, _, cat, search string, _, _ int) ([]Asset, error) {
			gotCat, gotSearch = cat, search
			return nil, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.ListAssets(ctxWithTenant(tenantID), &inventoryv1.ListAssetsRequest{CategoryId: "cam", Search: "arri"})
	require.NoError(t, err)
	assert.Equal(t, "cam", gotCat)
	assert.Equal(t, "arri", gotSearch)
}
```

- [ ] **Step 4: Gate + commit**

```bash
go test ./services/inventory/... -race && go build ./... && go vet ./services/inventory/ && gofmt -l services/inventory/*.go
git add services/inventory/service.go services/inventory/handler.go services/inventory/handler_test.go services/inventory/service_test.go
git commit -m "feat(inventory): ListAssets category/search + ListCheckouts limit/after (#192)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Migration `inventory/002` (5 damage cols) + down + no-001-down note → Task 1 ✅
- CheckinAsset SQL damage + GetActiveCheckout wire + repo sig changes → Task 1; service resolve-from-asset + status behavior → Task 3 ✅
- ListAssets category/search (SQL + repo) → Task 1; service/handler → Task 4 ✅
- ListCheckouts limit/after keyset (SQL + repo) → Task 1; service/handler + bad-cursor InvalidArgument → Task 4 ✅
- Proto: CheckInAssetRequest deprecate checkout_id + damage, AssetCheckout damage fields, ListAssetsRequest category/search, ListCheckoutsRequest limit/after → Task 2 ✅
- Perms (checkout / read / read), gate order, money=decimal, repair_cost negative reject, report_damage requires severity → Tasks 3/4 ✅
- Whole-tree vet after interface change; both doubles updated → Task 1 (+ every later task builds tree-wide) ✅
- checkout_id never removed (deprecate) → Task 2 ✅
- ListAssets dead `after` left alone (non-goal) → not touched ✅

**Placeholder scan:** none — migration, SQL, sigs, proto, service, handler, tests all concrete. The "confirm generated type / next field number / existing helper name" notes are compiler-checked steps, not placeholders.

**Type consistency:** `CheckInInput` (Task 1) == the field set built in the Task 3 handler == the service param. `Repository.CheckInAsset(...) (*AssetCheckout, error)` (Task 1) == `Service.CheckInAsset` call (Task 3). `ListAssets(status, categoryID, search, limit, offset)` and `ListCheckouts(assetID, limit, after)` signatures are identical across repo (Task 1), service (Tasks 1 stub → 4 final), and handler (Task 4). `AssetCheckout` damage fields (Task 1 model) == `checkoutToProto` mapping (Task 3) == proto fields (Task 2). `ErrNoActiveCheckout` (Task 1) mapped in grpcErr (Task 3).

**Ordering:** Task 1 (data, makes tree compile) → Task 2 (proto, independent) → Task 3 (checkin logic; needs 1+2, updates shared `checkoutToProto`) → Task 4 (list logic; needs 1+2, and Task 3's converter already landed). Each task builds tree-wide.
