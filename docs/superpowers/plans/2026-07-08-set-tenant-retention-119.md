# SetTenantRetention (#119) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an iam gRPC RPC `SetTenantRetention` that applies a status-preserving legal hold (indefinite pause or dated extension) to a suspended tenant, reusing the migration-017 `freeze_reason`/`hold_until` columns.

**Architecture:** Three layers, bottom-up: (1) a new repository write path `SetTenantLegalHold` that writes only the two hold columns; (2) a service method `SetTenantRetention` holding all domain guards (status-eligible, future `hold_until`, hold-collision) + audit; (3) proto RPC + handler wiring. Resume/un-pause is already `ClearTenantLegalHold`; force-advance is out of scope. **No migration, no sweeper/query change** — the sweeper's `ListTenantsDueForLifecycle` already skips held tenants.

**Tech Stack:** Go 1.22+, gRPC (`buf generate`), sqlc (`sqlc generate`), pgx v5 / pgtype, testify, `pkg/audit`, `pkg/interceptor`, `pkg/testdb`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-08-set-tenant-retention-119-design.md`.
- Coverage floor: **iam ≥ 85%** (`go test -coverprofile`).
- Widening the `iam.Repository` interface requires updating **all three** implementers — `*Postgres`, `*mockRepo`, and the e2e double `*iamRepo`. Only the first two have compile-time `var _ iam.Repository` assertions, so **`go vet ./...` (whole tree)** is the gate that catches the e2e double — `go build ./services/iam/...` alone will NOT.
- Errcheck runs in CI Lint: check every returned `error`.
- Logging via `slog`, no PII/secrets. Monetary rule N/A (no money here).
- Commits: Conventional Commits, scopes `iam` / `proto`. End commit messages with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Status constants (verified `services/iam/lifecycle.go:40-44`): `TenantStatusActive="active"`, `TenantStatusSuspended="suspended"`, `TenantStatusGrace="grace"`, `TenantStatusDeactivated="deactivated"`, `TenantStatusPurgeEligible="purge_eligible"`.
- Domain hold fields (`services/iam/models.go:52-53`): `HoldUntil *time.Time`, `FreezeReason *string`.

---

### Task 1: Repository write path — `SetTenantLegalHold`

**Files:**
- Modify: `services/iam/db/queries.sql` (add query near `ClearTenantLegalHold`, ~`:64`)
- Regenerate: `services/iam/db/queries.sql.go` (via `sqlc generate`)
- Modify: `services/iam/repository.go` (interface method, `// Tenants` section after `ClearTenantLegalHold`, ~`:57`)
- Modify: `services/iam/db/postgres.go` (impl near `ClearTenantLegalHold`, ~`:465`)
- Modify: `services/iam/service_test.go` (`mockRepo` field + method, ~`:31`/`:159`)
- Modify: `e2e/critical_path/helpers_test.go` (`iamRepo` method, after ~`:211`)
- Test: `services/iam/db/tenant_legal_hold_integration_test.go` (add test)

**Interfaces:**
- Produces: `Repository.SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error)`
- Produces: generated `(*Queries).SetTenantLegalHold(ctx, SetTenantLegalHoldParams) (Tenant, error)` with `SetTenantLegalHoldParams{ID uuid.UUID; HoldUntil pgtype.Timestamptz; FreezeReason pgtype.Text}`
- Produces: `mockRepo.setTenantLegalHoldFn` field (used by Task 2 & 3 tests)

- [ ] **Step 1: Add the sqlc query**

In `services/iam/db/queries.sql`, after the `ClearTenantLegalHold` query:

```sql
-- name: SetTenantLegalHold :one
-- Sets the two legal-hold columns on a tenant WITHOUT touching status or the
-- suspended_at/deactivated_at anchors — the operator override that pauses or
-- extends the retention sweeper for an already-suspended tenant (#119).
-- Unlike ClearTenantLegalHold this writes the columns: a NULL hold_until is an
-- indefinite hold, a future hold_until is a dated extension. Collision and
-- status-eligibility are enforced in the service layer, not here.
UPDATE tenants SET
    hold_until    = $2,
    freeze_reason = $3
WHERE id = $1
RETURNING *;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: `services/iam/db/queries.sql.go` gains `func (q *Queries) SetTenantLegalHold(ctx context.Context, arg SetTenantLegalHoldParams) (Tenant, error)` and a `SetTenantLegalHoldParams` struct. No error.

- [ ] **Step 3: Add the interface method**

In `services/iam/repository.go`, `// Tenants` section, immediately after the `ClearTenantLegalHold` method:

```go
	// SetTenantLegalHold sets the tenant's hold_until and freeze_reason to the
	// given values and returns the updated row. Status, suspended_at, and
	// deactivated_at are NOT touched — this applies a hold without regressing
	// the retention clock (#119). freezeReason is written verbatim (the caller
	// guarantees it is non-empty); a nil holdUntil writes NULL (indefinite
	// hold). Returns ErrTenantNotFound if the row is missing.
	SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error)
```

- [ ] **Step 4: Implement on `*Postgres`**

In `services/iam/db/postgres.go`, after the `ClearTenantLegalHold` method:

```go
func (p *Postgres) SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*iam.Tenant, error) {
	row, err := p.q.SetTenantLegalHold(ctx, SetTenantLegalHoldParams{
		ID:           id,
		HoldUntil:    pgTimestamptzFromTimePtr(holdUntil),
		FreezeReason: pgtype.Text{String: freezeReason, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrTenantNotFound
		}
		return nil, fmt.Errorf("iam/db: set tenant legal hold: %w", err)
	}
	return dbTenantToDomain(row), nil
}
```

(`pgtype`, `pgx`, `errors`, `fmt` are already imported in this file; `pgTimestamptzFromTimePtr` is at `postgres.go:986`.)

- [ ] **Step 5: Add the method to both test doubles**

In `services/iam/service_test.go`, add a field to the `mockRepo` struct (near `clearTenantLegalHoldFn`, ~`:52`):

```go
	setTenantLegalHoldFn         func(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error)
```

and the method (near the `ClearTenantLegalHold` mock impl, ~`:159`):

```go
func (m *mockRepo) SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
	if m.setTenantLegalHoldFn != nil {
		return m.setTenantLegalHoldFn(ctx, id, holdUntil, freezeReason)
	}
	return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
}
```

In `e2e/critical_path/helpers_test.go`, after the `iamRepo.ClearTenantLegalHold` method (~`:211`):

```go
func (r *iamRepo) SetTenantLegalHold(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*iam.Tenant, error) {
	t, ok := r.tenants[id]
	if !ok {
		return nil, iam.ErrTenantNotFound
	}
	t.HoldUntil = holdUntil
	t.FreezeReason = &freezeReason
	return t, nil
}
```

- [ ] **Step 6: Write the failing integration test**

In `services/iam/db/tenant_legal_hold_integration_test.go`: add `"github.com/jackc/pgx/v5/pgtype"` to the import block, then append:

```go
func TestSetTenantLegalHold_AppliesIndefiniteHold_SkipsSweeper(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	// Unheld suspended tenant is a sweeper candidate.
	id := insertSuspendedTenant(t, tx, "To-Hold Studios", nil, nil)
	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(), Limit: 100,
	})
	require.NoError(t, err)
	require.True(t, containsTenant(rows, id), "sanity: unheld tenant is a candidate before hold")

	// Apply an indefinite hold via the new write path.
	held, err := q.SetTenantLegalHold(context.Background(), iamdb.SetTenantLegalHoldParams{
		ID:           id,
		HoldUntil:    pgtype.Timestamptz{}, // NULL = indefinite
		FreezeReason: pgtype.Text{String: "support escalation", Valid: true},
	})
	require.NoError(t, err)
	assert.True(t, held.FreezeReason.Valid)
	assert.Equal(t, "support escalation", held.FreezeReason.String)
	assert.False(t, held.HoldUntil.Valid, "indefinite hold => hold_until NULL")
	assert.Equal(t, "suspended", held.Status, "status must be unchanged by a hold write")

	// Sweeper now skips it.
	rows, err = q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(), Limit: 100,
	})
	require.NoError(t, err)
	assert.False(t, containsTenant(rows, id), "held tenant must be skipped by the sweeper")
}
```

- [ ] **Step 7: Compile the whole tree + vet (catches the e2e double)**

Run: `go build ./... && go vet ./...`
Expected: PASS with no output. (If `go vet` complains that `*iamRepo` does not implement `iam.Repository`, Step 5's e2e method is missing or mis-typed.)

- [ ] **Step 8: Run the integration test** (requires Docker/Postgres)

Run: `go test ./services/iam/db/ -tags=integration -run TestSetTenantLegalHold_AppliesIndefiniteHold_SkipsSweeper -v`
Expected: PASS. (If Docker is unavailable in this environment, note it and rely on Step 7 + CI's "Integration Tests (real Postgres)" job.)

- [ ] **Step 9: Commit**

```bash
git add services/iam/db/queries.sql services/iam/db/queries.sql.go services/iam/repository.go \
        services/iam/db/postgres.go services/iam/service_test.go e2e/critical_path/helpers_test.go \
        services/iam/db/tenant_legal_hold_integration_test.go
git commit -m "feat(iam): add SetTenantLegalHold repo write path (#119)

Status-preserving write of hold_until/freeze_reason for an already-suspended
tenant. Widens iam.Repository; updates Postgres + mockRepo + e2e iamRepo doubles.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Service — `SetTenantRetention` + error sentinels

**Files:**
- Modify: `services/iam/errors.go` (three sentinels, in the `var (...)` block, ~`:11`)
- Create: `services/iam/retention_override.go`
- Test: `services/iam/retention_override_test.go`

**Interfaces:**
- Consumes: `Repository.SetTenantLegalHold(...)` and `mockRepo.setTenantLegalHoldFn` (Task 1); `mustMarshalHoldState(*Tenant)` (`lifecycle.go:194`); `newTestService`, `mockRepo`, `memoryAuditStore`, `fixedTenantID` (test helpers).
- Produces: `(*Service).SetTenantRetention(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string, overwrite bool) (*Tenant, error)`
- Produces: sentinels `ErrTenantNotHoldable`, `ErrTenantHoldExists`, `ErrHoldUntilInPast` (consumed by Task 3's `grpcError`).

- [ ] **Step 1: Add the sentinels**

In `services/iam/errors.go`, inside the `var (...)` block:

```go
	// ErrTenantNotHoldable is returned by SetTenantRetention when the tenant's
	// status has no running retention clock to hold — 'active' (not yet
	// suspended) or 'purge_eligible' (terminal). Maps to FailedPrecondition.
	ErrTenantNotHoldable = errors.New("iam: tenant status has no retention clock to hold")
	// ErrTenantHoldExists is returned by SetTenantRetention when the tenant
	// already has an active hold and overwrite was not requested. Maps to
	// FailedPrecondition. The wrapped message names the existing freeze_reason.
	ErrTenantHoldExists = errors.New("iam: tenant already has an active hold; pass overwrite to replace it")
	// ErrHoldUntilInPast is returned by SetTenantRetention when hold_until is at
	// or before now. Maps to InvalidArgument.
	ErrHoldUntilInPast = errors.New("iam: hold_until must be in the future")
```

- [ ] **Step 2: Write the failing service tests**

Create `services/iam/retention_override_test.go`:

```go
package iam

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

func TestSetTenantRetention_IndefinitePause(t *testing.T) {
	t.Parallel()
	var gotHold *time.Time
	var gotReason string
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			gotHold, gotReason = holdUntil, freezeReason
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
		},
	}
	got, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "support escalation", false)
	require.NoError(t, err)
	assert.Nil(t, gotHold, "indefinite pause passes nil hold_until")
	assert.Equal(t, "support escalation", gotReason)
	require.NotNil(t, got.FreezeReason)
	assert.Equal(t, "support escalation", *got.FreezeReason)
}

func TestSetTenantRetention_ExtendUntilFutureDate(t *testing.T) {
	t.Parallel()
	future := time.Now().UTC().Add(60 * 24 * time.Hour)
	var gotHold *time.Time
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusGrace}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			gotHold = holdUntil
			return &Tenant{ID: id, Status: TenantStatusGrace, HoldUntil: holdUntil, FreezeReason: &freezeReason}, nil
		},
	}
	got, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, &future, "retention-extended: ticket-42", false)
	require.NoError(t, err)
	require.NotNil(t, gotHold)
	assert.Equal(t, future, *gotHold)
	assert.Equal(t, TenantStatusGrace, got.Status, "status must not regress")
}

func TestSetTenantRetention_RejectsNonHoldableStatus(t *testing.T) {
	t.Parallel()
	for _, st := range []string{TenantStatusActive, TenantStatusPurgeEligible} {
		st := st
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			repo := &mockRepo{
				getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
					return &Tenant{ID: id, Status: st}, nil
				},
				setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) {
					t.Fatal("SetTenantLegalHold must not be called for non-holdable status")
					return nil, nil
				},
			}
			_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "x", false)
			assert.ErrorIs(t, err, ErrTenantNotHoldable)
		})
	}
}

func TestSetTenantRetention_RejectsPastHoldUntil(t *testing.T) {
	t.Parallel()
	past := time.Now().UTC().Add(-time.Hour)
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) {
			t.Fatal("SetTenantLegalHold must not be called when hold_until is in the past")
			return nil, nil
		},
	}
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, &past, "x", false)
	assert.ErrorIs(t, err, ErrHoldUntilInPast)
}

func TestSetTenantRetention_CollisionRejectedWithoutOverwrite(t *testing.T) {
	t.Parallel()
	existing := "legal:case-42"
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &existing}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) {
			t.Fatal("must not overwrite an existing hold without overwrite=true")
			return nil, nil
		},
	}
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "retention-extended", false)
	assert.ErrorIs(t, err, ErrTenantHoldExists)
	assert.Contains(t, err.Error(), existing, "error must surface the existing freeze_reason")
}

func TestSetTenantRetention_CollisionAllowedWithOverwrite(t *testing.T) {
	t.Parallel()
	existing := "legal:case-42"
	called := false
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &existing}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			called = true
			return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &freezeReason}, nil
		},
	}
	_, err := newTestService(repo).SetTenantRetention(context.Background(), fixedTenantID, nil, "retention-extended", true)
	require.NoError(t, err)
	assert.True(t, called, "overwrite=true must proceed to the repo write")
}

func TestSetTenantRetention_EmitsAuditWithOverwriteMeta(t *testing.T) {
	// Not t.Parallel() — waits on the audit flush.
	existing := "legal:case-42"
	before := &Tenant{ID: fixedTenantID, Status: TenantStatusSuspended, FreezeReason: &existing}
	newReason := "retention-extended: ticket-42"
	after := &Tenant{ID: fixedTenantID, Status: TenantStatusSuspended, FreezeReason: &newReason}
	repo := &mockRepo{
		getTenantFn:          func(_ context.Context, _ uuid.UUID) (*Tenant, error) { return before, nil },
		setTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID, _ *time.Time, _ string) (*Tenant, error) { return after, nil },
	}
	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{BufferSize: 10, FlushInterval: 10 * time.Millisecond, BatchSize: 10}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	actorID := uuid.MustParse("a2000000-0000-0000-0000-000000000119")
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: actorID, Email: "admin@platform.internal", IP: "10.0.0.9"})

	_, err := svc.SetTenantRetention(ctx, fixedTenantID, nil, newReason, true)
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, audit.ActionLegalHoldApplied, e.Action)
	assert.Equal(t, audit.ResourceTenant, e.ResourceType)
	assert.Equal(t, fixedTenantID, e.TenantID)
	assert.Equal(t, actorID, e.ActorID)
	assert.JSONEq(t, `{"overwrote_previous":true,"previous_reason":"legal:case-42"}`, string(e.Metadata))
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./services/iam/ -run TestSetTenantRetention -v`
Expected: FAIL — compile error `svc.SetTenantRetention undefined` (method not yet written).

- [ ] **Step 4: Implement the service**

Create `services/iam/retention_override.go`:

```go
package iam

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

// isHoldableStatus reports whether a tenant's status has a running retention
// clock that an operator hold can pause or extend. 'active' has no clock;
// 'purge_eligible' is terminal.
func isHoldableStatus(status string) bool {
	switch status {
	case TenantStatusSuspended, TenantStatusGrace, TenantStatusDeactivated:
		return true
	default:
		return false
	}
}

// SetTenantRetention applies a status-preserving legal hold to a suspended
// tenant, pausing the retention sweeper (#119). A nil holdUntil is an
// indefinite pause; a future holdUntil extends the hold until that time.
// freezeReason must be non-empty (the handler enforces this). If the tenant
// already has an active hold and overwrite is false the call is rejected with
// ErrTenantHoldExists; the returned error names the existing freeze_reason.
func (s *Service) SetTenantRetention(
	ctx context.Context,
	id uuid.UUID,
	holdUntil *time.Time,
	freezeReason string,
	overwrite bool,
) (*Tenant, error) {
	before, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("iam: set tenant retention %s: %w", id, err)
	}

	if !isHoldableStatus(before.Status) {
		return nil, fmt.Errorf("iam: set tenant retention %s (status %s): %w", id, before.Status, ErrTenantNotHoldable)
	}

	if holdUntil != nil && !holdUntil.After(time.Now().UTC()) {
		return nil, fmt.Errorf("iam: set tenant retention %s: %w", id, ErrHoldUntilInPast)
	}

	overwrote := before.FreezeReason != nil && *before.FreezeReason != ""
	if overwrote && !overwrite {
		return nil, fmt.Errorf("iam: set tenant retention %s (existing hold %q): %w", id, *before.FreezeReason, ErrTenantHoldExists)
	}

	after, err := s.repo.SetTenantLegalHold(ctx, id, holdUntil, freezeReason)
	if err != nil {
		return nil, fmt.Errorf("iam: set tenant retention %s: %w", id, err)
	}

	if s.audit != nil {
		actor, _ := audit.ActorFromContext(ctx)
		var prevReason *string
		if overwrote {
			prevReason = before.FreezeReason
		}
		s.audit.Log(audit.Event{
			TenantID:     id,
			ActorID:      actor.UserID,
			ActorEmail:   actor.Email,
			ActorIP:      actor.IP,
			Action:       audit.ActionLegalHoldApplied,
			ResourceType: audit.ResourceTenant,
			ResourceID:   id,
			OldState:     mustMarshalHoldState(before),
			NewState:     mustMarshalHoldState(after),
			Metadata:     mustMarshalRetentionMeta(overwrote, prevReason),
			OccurredAt:   time.Now().UTC(),
		})
	}

	return after, nil
}

// mustMarshalRetentionMeta encodes the overwrite context for a
// SetTenantRetention audit event. previous_reason is omitted when nothing was
// overwritten. Panics only on a marshal bug (mirrors the mustMarshal* helpers
// in lifecycle.go).
func mustMarshalRetentionMeta(overwrote bool, previousReason *string) json.RawMessage {
	b, err := json.Marshal(struct {
		OverwrotePrevious bool    `json:"overwrote_previous"`
		PreviousReason    *string `json:"previous_reason,omitempty"`
	}{overwrote, previousReason})
	if err != nil {
		panic(fmt.Sprintf("iam: marshal retention override metadata: %v", err))
	}
	return b
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/iam/ -run TestSetTenantRetention -v`
Expected: PASS (all seven test functions, including the sub-tests).

- [ ] **Step 6: Commit**

```bash
git add services/iam/errors.go services/iam/retention_override.go services/iam/retention_override_test.go
git commit -m "feat(iam): SetTenantRetention service — hold/extend guards + audit (#119)

Status-eligible, future-hold_until, and hold-collision guards; emits
legal_hold_applied audit with overwrite metadata. Reuses SetTenantLegalHold.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Proto + handler + error mapping

**Files:**
- Modify: `proto/thittam/iam/v1/iam.proto` (RPC in the `// --- Tenants ---` block ~`:52-61`; message near `ClearTenantLegalHoldRequest` ~`:331`)
- Regenerate: `gen/iam/v1/*` (via `buf generate`)
- Modify: `services/iam/handler.go` (handler method after `ClearTenantLegalHold` ~`:430`; two cases in `grpcError` ~`:649-690`)
- Test: `services/iam/handler_test.go` (append near the `ClearTenantLegalHold` handler tests ~`:607`)

**Interfaces:**
- Consumes: `(*Service).SetTenantRetention(...)` and sentinels `ErrTenantNotHoldable`/`ErrTenantHoldExists`/`ErrHoldUntilInPast` (Task 2); `platformAdminCtx()`, `newHandler()`, `newTestService`, `mockRepo` (test helpers).
- Produces: `SetTenantRetentionRequest` proto message + `(*Handler).SetTenantRetention`.

- [ ] **Step 1: Add the proto RPC + message**

In `proto/thittam/iam/v1/iam.proto`, add to the `// --- Tenants ---` block, after the `ClearTenantLegalHold` rpc:

```proto
  // SetTenantRetention applies a status-preserving legal hold to a suspended
  // tenant — an indefinite pause (hold_until unset) or a dated extension
  // (hold_until in the future). Reuses the freeze_reason/hold_until columns;
  // the retention sweeper skips held tenants. Resume is ClearTenantLegalHold.
  // Platform-admin only (#119).
  rpc SetTenantRetention(SetTenantRetentionRequest) returns (Tenant);
```

and add the request message near `ClearTenantLegalHoldRequest`:

```proto
message SetTenantRetentionRequest {
  string id = 1;
  // Required, non-empty. Written to freeze_reason; its presence freezes the
  // sweeper. Encodes the operator's rationale (e.g. "retention-extended: ...").
  string freeze_reason = 2;
  // Unset = indefinite pause. Set = extend the hold until this time; the
  // service rejects a value at or before now.
  optional google.protobuf.Timestamp hold_until = 3;
  // Replace an existing active hold. Default false → the call is rejected with
  // FAILED_PRECONDITION if the tenant already has a hold.
  bool overwrite = 4;
}
```

(`google/protobuf/timestamp.proto` is already imported — `SuspendTenantRequest` uses it.)

- [ ] **Step 2: Regenerate proto + verify it's additive**

Run: `buf lint && buf generate && buf breaking proto --against '.git#branch=main,subdir=proto'`
Expected: no lint errors; `gen/iam/v1/iam.pb.go` gains `SetTenantRetentionRequest`, `gen/iam/v1/iam_grpc.pb.go` gains `SetTenantRetention` on `IAMServiceServer`; `buf breaking` reports **no breaking changes** (RPC + message are additive). `go build ./...` still passes because `Handler` embeds `UnimplementedIAMServiceServer`.

- [ ] **Step 3: Write the failing handler tests**

In `services/iam/handler_test.go`, append:

```go
func TestHandler_SetTenantRetention_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended"}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended", FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
		},
	}))
	resp, err := h.SetTenantRetention(platformAdminCtx(), &iamv1.SetTenantRetentionRequest{
		Id: tenantID.String(), FreezeReason: "support escalation",
	})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetId())
}

func TestHandler_SetTenantRetention_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SetTenantRetention(context.Background(), &iamv1.SetTenantRetentionRequest{
		Id: uuid.New().String(), FreezeReason: "x",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SetTenantRetention_EmptyReason(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SetTenantRetention(platformAdminCtx(), &iamv1.SetTenantRetentionRequest{
		Id: uuid.New().String(), FreezeReason: "  ",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetTenantRetention_NotHoldableMapsFailedPrecondition(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "active"}, nil
		},
	}))
	_, err := h.SetTenantRetention(platformAdminCtx(), &iamv1.SetTenantRetentionRequest{
		Id: uuid.New().String(), FreezeReason: "x",
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./services/iam/ -run TestHandler_SetTenantRetention -v`
Expected: FAIL — compile error `h.SetTenantRetention undefined`.

- [ ] **Step 5: Implement the handler + error mapping**

In `services/iam/handler.go`, after the `ClearTenantLegalHold` handler:

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

In the same file, in `grpcError` (~`:649`), add these cases before the `default:` — and add `ErrHoldUntilInPast` to the existing `InvalidArgument` case:

```go
	case errors.Is(err, ErrTenantNotHoldable),
		errors.Is(err, ErrTenantHoldExists):
		return status.Error(codes.FailedPrecondition, err.Error())
```

Update the existing InvalidArgument case from:

```go
	case errors.Is(err, ErrInvalidPlan),
		errors.Is(err, ErrRoleNotProjectScoped):
		return status.Error(codes.InvalidArgument, err.Error())
```

to:

```go
	case errors.Is(err, ErrInvalidPlan),
		errors.Is(err, ErrRoleNotProjectScoped),
		errors.Is(err, ErrHoldUntilInPast):
		return status.Error(codes.InvalidArgument, err.Error())
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./services/iam/ -run TestHandler_SetTenantRetention -v`
Expected: PASS (all four).

- [ ] **Step 7: Full iam suite + whole-tree build/vet + coverage**

Run: `go build ./... && go vet ./... && go test ./services/iam/... -short -coverprofile=cover.out && go tool cover -func=cover.out | tail -1`
Expected: build/vet clean; all iam unit tests pass; total coverage ≥ 85%.

- [ ] **Step 8: Commit**

```bash
git add proto/thittam/iam/v1/iam.proto gen/iam/v1/ services/iam/handler.go services/iam/handler_test.go
git commit -m "feat(iam): SetTenantRetention RPC + handler (#119)

Additive proto RPC + status-preserving hold/extend handler wired to the
service; maps not-holdable/hold-exists to FailedPrecondition and past
hold_until to InvalidArgument.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Proto RPC + `SetTenantRetentionRequest` (id/freeze_reason/hold_until/overwrite) → Task 3 Step 1. ✓
- Handler mirroring `SuspendTenant` (role gate, empty-reason, presence-preserving hold_until) → Task 3 Step 5. ✓
- Service guards: status-eligible, future hold_until, collision-reject-unless-overwrite → Task 2 Step 4 + tests Step 2. ✓
- Audit `ActionLegalHoldApplied` with `{overwrote_previous, previous_reason}` metadata → Task 2 Step 4 (`mustMarshalRetentionMeta`) + audit test. ✓
- Repo `SetTenantLegalHold` (status/anchors untouched) → Task 1. ✓
- Error mapping (`FailedPrecondition` / `InvalidArgument`) → Task 3 Step 5 (`grpcError`). Note: spec §5 said "errors.go" for mapping; corrected to `handler.go` where `grpcError` actually lives — sentinels stay in `errors.go`. ✓
- Tests: service unit matrix, handler role-gate + empty-reason, repo integration → Tasks 1–3. ✓
- Non-goals (no migration, no force-advance, no proto Tenant field, no sweeper change) → honored; no task touches migrations or the sweeper query. ✓
- Three-implementer interface widening + whole-tree `go vet` → Task 1 Steps 5 & 7. ✓

**Placeholder scan:** none — every code/step is concrete.

**Type consistency:** `SetTenantLegalHold(ctx, uuid.UUID, *time.Time, string) (*Tenant, error)` and `SetTenantRetention(ctx, uuid.UUID, *time.Time, string, bool) (*Tenant, error)` are used identically across interface, impl, mocks, service, handler, and tests. `mockRepo.setTenantLegalHoldFn` signature matches. Sentinel names consistent across Tasks 2 and 3. `mustMarshalRetentionMeta` defined and called in the same file.
