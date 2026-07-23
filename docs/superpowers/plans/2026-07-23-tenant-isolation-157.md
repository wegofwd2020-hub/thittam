# Tenant Isolation on Child-Row Queries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close 14 tenant-isolation defects in `project`, `budget` and `inventory` by threading `tenantID` through the repository signatures that reach child-row tables, so a caller in tenant A can no longer read, update or delete rows in tenant B by supplying their UUID.

**Architecture:** Every affected query gains an `AND tenant_id = $N` predicate, and every repository method that runs one gains a `tenantID uuid.UUID` parameter. This matches `GetProduction(ctx, tenantID, id)` and `GetJournalEntry(ctx, tenantID, id)`, which already have that shape. A required parameter cannot be forgotten — the guard-by-type principle. Handlers already hold the tenant; they pass it down. No migration: every affected table already declares `tenant_id UUID NOT NULL`.

**Tech Stack:** Go 1.25, sqlc v1.26.0, pgx/v5, grpc-go, testify, `github.com/google/uuid`, `shopspring/decimal`.

**Spec:** `docs/superpowers/specs/2026-07-23-tenant-isolation-157-design.md` (committed on this branch at `bf15aa8`). Read §2 before starting — it explains which unscoped queries are defects and which are correct.

## Global Constraints

- **Branch:** `fix/tenant-isolation-157`, already created, base `e1871c5`.
- **No Docker. No database.** NEVER run `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it destroyed unrelated MinIO dev data once. For any DB verification use a disposable, uniquely-named throwaway container, or `pkg/testdb` (integration tests SKIP without `THITTAM_TEST_DSN`). CI's real-Postgres job is the authoritative gate.
- **`go vet ./...` (whole tree) is the completion gate for every task.** `go build` and per-package tests are NOT sufficient — see "The hidden implementers" below.
- **A signature change and every one of its call sites land in the SAME commit.** A commit that does not build makes the branch un-bisectable. (Lesson from #149 Task 3.)
- **Regenerate with `make generate-sqlc`** after any `queries.sql` edit, and commit `queries.sql.go` in the same commit. `sqlc` v1.26.0 is installed.
- **Do NOT add any permission gate in this plan.** This branch is tenant isolation only. In particular do NOT gate `inventory.ListCheckouts` on `inventory:read` — that string is granted to `inventory_manager` alone, not even `super_admin`, so gating it locks out every role that can check an asset out. Deferred to #139 slice D.
- **No migration. No proto change. No new permission string.** `git diff --stat gen/` (protobuf output) must be empty. `services/*/db/queries.sql.go` (sqlc output) WILL change — that is expected.
- `errcheck` runs in CI; `golangci-lint` is not installed locally. A deferred rollback needs `defer func() { _ = tx.Rollback(ctx) }()`.
- Coverage floors: `project`, `budget`, `inventory` are all in the `others ≥ 75%` tier. Record the baseline before each task; do not regress.
- Monetary values are always `decimal.Decimal` / `NUMERIC(14,2)`, never `float64`.
- Commits follow Conventional Commits with scopes `project`, `budget`, `inventory`.

### The hidden implementers

Widening a `Repository` method breaks every implementer, and they are not all next to the service:

| interface | implementers |
|---|---|
| `project.Repository` | `db.Postgres`; `mockRepo` (`services/project/service_test.go`); **`projectMock`** and **`phaseReturnMock`** (`tests/integration/vertical/mocks_test.go`) |
| `budget.Repository` | `db.Postgres`; `mockRepo` (`services/budget/service_test.go`); **`budgetMock`** (`tests/integration/vertical/mocks_test.go`) |
| `inventory.Repository` | `db.Postgres`; `mockRepo` (`services/inventory/service_test.go`); **`inventoryMock`** (`tests/integration/vertical/mocks_test.go`) |

`tests/integration/vertical/mocks_test.go` is in a different tree entirely and holds doubles for all three services. This is the same trap that `iam.Repository` sprang — caught only by CI. **`go vet ./...` compiles test files in every package and is the only sufficient local signal.**

### The mock defaults trap

Unset fn-fields do NOT uniformly return benign zero values:

- `project.mockRepo.GetPhase` returns `&Phase{ID: id, PhaseType: "development", Status: "active"}` — a usable phase.
- `budget.mockRepo.GetLineItem` returns `&BudgetLineItem{ID: id, CategoryID: "above_the_line"}`.
- `inventory.mockRepo.GetCheckout` returns `&AssetCheckout{ID: id}`.
- `project.mockRepo.ListPhases` and `RemoveCrewMember` have **no fn-field at all** — hardcoded returns.

So a denial test asserting only a status code can pass against vulnerable code. **Every new test in this plan asserts the `tenantID` the repository actually received.**

### Verifying a test has teeth

Because signatures change, the parent commit will not compile against new tests, so the usual `git worktree` teeth check does not apply directly. Use the predicate-revert check instead.

**⚠️ RUN THIS ONLY AFTER THE TASK'S COMMIT EXISTS.** The restore step is `git checkout <file>`, which restores from HEAD — so if the fix is still uncommitted when you run it, **it reverts the real fix, not just the experiment.** That happened on Task 1: the implementer caught it and re-applied by hand, but do not repeat it. Commit first, experiment second.

1. Implement the task fully; tests pass; **commit** (the task's commit step).
2. In the working tree, delete ONLY the `AND tenant_id = $N` from one query in `queries.sql`, leaving all Go signatures intact.
3. `make generate-sqlc`
4. Re-run that query's cross-tenant test and record the result.
5. `git checkout services/<svc>/db/queries.sql && make generate-sqlc` to restore — now safe, because HEAD holds the committed fix.
6. Confirm `git status --short` is clean and `git log --oneline -1` still shows your task commit.

**Expected result, and why it is not a failure:** the test will most likely still PASS. These are handler tests driven by mocks, so the mock answers the call and Postgres is never involved — removing a SQL predicate cannot affect them. Record that outcome plainly. It means the handler tests prove the *tenant is threaded through the Go call chain*, and CI's real-Postgres integration job is what proves the *SQL predicate*. Do not "fix" a test to make this check fail; do not claim teeth the test does not have.

Report the result in the task report. If it is impractical for a given test, say so explicitly — do not skip it silently.

**`make generate-sqlc` is repo-wide.** It regenerates every service and will dirty `services/billing/`, which carries pre-existing drift unrelated to this branch (tracked as #160). Revert those files (`git checkout services/billing/`) before committing so the security diff stays scoped. Never `git add -A`.

---

### Task 1: project — phase queries

Scopes the three phase queries, adds the two missing `Service` methods, and routes the handler through them instead of `h.svc.repo.*`.

**Files:**
- Modify: `services/project/db/queries.sql` (ListPhases, GetPhase, UpdatePhaseStatus)
- Modify: `services/project/db/queries.sql.go` (regenerated — do not hand-edit)
- Modify: `services/project/db/postgres.go:134-170`
- Modify: `services/project/repository.go:20-22`
- Modify: `services/project/service.go` (add `GetPhase`, `ListPhases`; change `UpdatePhaseStatus`)
- Modify: `services/project/handler.go:216, 223-245, 247-266`
- Modify: `services/project/service_test.go` (mockRepo)
- Modify: `tests/integration/vertical/mocks_test.go:25-27, 36-38`
- Test: `services/project/handler_test.go`

**Interfaces:**
- Produces: `Repository.ListPhases(ctx, tenantID, productionID uuid.UUID, limit, offset int) ([]Phase, error)`, `Repository.GetPhase(ctx, tenantID, id uuid.UUID) (*Phase, error)`, `Repository.UpdatePhaseStatus(ctx, tenantID, id uuid.UUID, status string) error`, `Service.GetPhase(ctx, tenantID, id uuid.UUID) (*Phase, error)`, `Service.ListPhases(ctx, tenantID, productionID uuid.UUID, limit, offset int) ([]Phase, error)`, `Service.UpdatePhaseStatus(ctx, tenantID, id uuid.UUID, newPhaseType string) error`
- Consumes: nothing from earlier tasks.

- [ ] **Step 1: Record the coverage baseline**

```bash
go test ./services/project/ -cover 2>&1 | tail -2
```

Write the percentage into the task report. You will compare against it in Step 12.

- [ ] **Step 2: Write the failing cross-tenant tests**

Add to `services/project/handler_test.go`. `tenantRecordingRepo` records what the repository was handed, which is the assertion that has teeth — a status-code-only check would pass against the unscoped code because `mockRepo.GetPhase` returns a usable phase by default.

```go
// tenantRecordingRepo records the tenant ID each phase query receives, so a
// test can prove the handler passed the CALLER's tenant and not something
// derived from the request. mockRepo's defaults return usable rows, so
// asserting only on the status code would pass against unscoped code.
type tenantRecordingRepo struct {
	mockRepo
	gotListTenant   uuid.UUID
	gotGetTenant    uuid.UUID
	gotUpdateTenant uuid.UUID
}

func (r *tenantRecordingRepo) ListPhases(ctx context.Context, tenantID, prodID uuid.UUID, limit, offset int) ([]Phase, error) {
	r.gotListTenant = tenantID
	return nil, nil
}

func (r *tenantRecordingRepo) GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*Phase, error) {
	r.gotGetTenant = tenantID
	return &Phase{ID: id, TenantID: tenantID, PhaseType: "development", Status: "active"}, nil
}

func (r *tenantRecordingRepo) UpdatePhaseStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	r.gotUpdateTenant = tenantID
	return nil
}

func TestHandler_ListPhases_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &tenantRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.ListPhases(ctxWithTenant(callerTenant), &projectv1.ListPhasesRequest{
		ProductionId: uuid.New().String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotListTenant,
		"ListPhases must query with the caller's tenant, not an unscoped query")
}

func TestHandler_UpdatePhaseStatus_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &tenantRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.UpdatePhaseStatus(ctxWithTenant(callerTenant), &projectv1.UpdatePhaseStatusRequest{
		PhaseId:      uuid.New().String(),
		NewPhaseType: "production",
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotUpdateTenant,
		"UpdatePhaseStatus must write with the caller's tenant")
	require.Equal(t, callerTenant, repo.gotGetTenant,
		"the read-back after the update must also be tenant-scoped")
}

func TestHandler_UpdatePhaseStatus_NoTenantUnauthenticated(t *testing.T) {
	t.Parallel()
	repo := &tenantRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	// callerCtx() carries a caller but NO tenant.
	_, err := h.UpdatePhaseStatus(callerCtx(), &projectv1.UpdatePhaseStatusRequest{
		PhaseId:      uuid.New().String(),
		NewPhaseType: "production",
	})

	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.Equal(t, uuid.Nil, repo.gotUpdateTenant,
		"the repository must not be reached without a tenant")
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./services/project/ -run 'PassesCallerTenant|NoTenantUnauthenticated' -v`
Expected: FAIL to compile — `unknown field gotListTenant` is not the error; the error is that `tenantRecordingRepo.ListPhases` does not satisfy the `Repository` interface, because the interface still declares the 4-argument form. That compile failure is the correct starting state.

- [ ] **Step 4: Add the tenant predicate to the three queries**

In `services/project/db/queries.sql`, replace the three phase queries:

```sql
-- name: ListPhases :many
SELECT * FROM phases WHERE production_id = $1 AND tenant_id = $2 ORDER BY created_at ASC LIMIT $3 OFFSET $4;

-- name: GetPhase :one
SELECT * FROM phases WHERE id = $1 AND tenant_id = $2;

-- name: UpdatePhaseStatus :one
UPDATE phases SET status = $2, updated_at = now()
WHERE id = $1 AND tenant_id = $3
RETURNING *;
```

- [ ] **Step 5: Regenerate sqlc**

```bash
make generate-sqlc
git diff --stat services/project/db/queries.sql.go
```

Expected: `queries.sql.go` shows changes to `ListPhasesParams`, `GetPhaseParams` (new struct — `GetPhase` previously took a bare `uuid.UUID`), and `UpdatePhaseStatusParams`.

- [ ] **Step 6: Update the Postgres implementation**

In `services/project/db/postgres.go`, replace the three methods:

```go
func (p *Postgres) GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*project.Phase, error) {
	row, err := p.q.GetPhase(ctx, GetPhaseParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, project.ErrPhaseNotFound
		}
		return nil, fmt.Errorf("project: get phase: %w", err)
	}
	return phaseFromDB(row), nil
}

func (p *Postgres) ListPhases(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]project.Phase, error) {
	rows, err := p.q.ListPhases(ctx, ListPhasesParams{
		ProductionID: productionID,
		TenantID:     tenantID,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("project: list phases: %w", err)
	}
	result := make([]project.Phase, len(rows))
	for i, row := range rows {
		result[i] = *phaseFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdatePhaseStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	_, err := p.q.UpdatePhaseStatus(ctx, UpdatePhaseStatusParams{ID: id, TenantID: tenantID, Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return project.ErrPhaseNotFound
		}
		return fmt.Errorf("project: update phase status: %w", err)
	}
	return nil
}
```

Note the generated field names may differ slightly; read `queries.sql.go` after Step 5 and match them exactly.

- [ ] **Step 7: Widen the Repository interface**

In `services/project/repository.go`, replace lines 20-22:

```go
	// Phases
	CreatePhase(ctx context.Context, p *Phase) error
	ListPhases(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]Phase, error)
	GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*Phase, error)
	UpdatePhaseStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error
```

- [ ] **Step 8: Add the Service methods and update UpdatePhaseStatus**

In `services/project/service.go`, add the two new pass-throughs next to the existing phase methods:

```go
// GetPhase returns a single phase, scoped to the caller's tenant.
func (s *Service) GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*Phase, error) {
	return s.repo.GetPhase(ctx, tenantID, id)
}

// ListPhases returns a production's phases, scoped to the caller's tenant.
func (s *Service) ListPhases(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]Phase, error) {
	return s.repo.ListPhases(ctx, tenantID, productionID, limit, offset)
}
```

Then replace `Service.UpdatePhaseStatus` entirely. **Only the signature and the two repository calls change** — every line of the vertical-config transition validation is preserved verbatim:

```go
func (s *Service) UpdatePhaseStatus(ctx context.Context, tenantID, phaseID uuid.UUID, newPhaseType string) error {
	vcfg := vertical.MustFromContext(ctx)

	phase, err := s.repo.GetPhase(ctx, tenantID, phaseID)
	if err != nil {
		return fmt.Errorf("get phase: %w", err)
	}

	// Look up current phase type in vertical config
	currentPT := vcfg.FindPhaseType(phase.PhaseType)
	if currentPT == nil {
		return fmt.Errorf("%w: current phase type %q not found", ErrInvalidPhaseType, phase.PhaseType)
	}

	// Validate transition is allowed
	if !currentPT.CanTransitionTo(newPhaseType) {
		return fmt.Errorf("%w: cannot transition from %q to %q", ErrInvalidTransition, phase.PhaseType, newPhaseType)
	}

	// Validate target phase type exists
	targetPT := vcfg.FindPhaseType(newPhaseType)
	if targetPT == nil {
		return fmt.Errorf("%w: target %q is not valid", ErrInvalidPhaseType, newPhaseType)
	}

	return s.repo.UpdatePhaseStatus(ctx, tenantID, phaseID, newPhaseType)
}
```

Note this closes a second hole in the same function: the `GetPhase` that feeds the transition check was itself unscoped, so the validation was reading another tenant's phase type.

- [ ] **Step 9: Update the handler — tenant block, then route through Service**

In `services/project/handler.go`:

`CreatePhase` (line ~216) already has `tenantID` in scope. Change only the read-back:

```go
	created, err := h.svc.GetPhase(ctx, tenantID, phase.ID)
```

`ListPhases` — add the tenant block above the existing gate, and route through the Service:

```go
func (h *Handler) ListPhases(ctx context.Context, req *projectv1.ListPhasesRequest) (*projectv1.ListPhasesResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	// Proto does not carry limit/offset yet — apply a server-side cap.
	const defaultLimit = 50 // productions rarely have more than 50 phases
	phases, err := h.svc.ListPhases(ctx, tenantID, productionID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*projectv1.Phase, len(phases))
	for i := range phases {
		out[i] = phaseToProto(&phases[i])
	}
	return &projectv1.ListPhasesResponse{Phases: out}, nil
}
```

`UpdatePhaseStatus` — same pattern:

```go
func (h *Handler) UpdatePhaseStatus(ctx context.Context, req *projectv1.UpdatePhaseStatusRequest) (*projectv1.Phase, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:write"); err != nil {
		return nil, err
	}

	phaseID, err := uuid.Parse(req.GetPhaseId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid phase ID")
	}

	if err := h.svc.UpdatePhaseStatus(ctx, tenantID, phaseID, req.GetNewPhaseType()); err != nil {
		return nil, grpcErr(err)
	}

	phase, err := h.svc.GetPhase(ctx, tenantID, phaseID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return phaseToProto(phase), nil
}
```

**After this step `grep -c 'h\.svc\.repo\.' services/project/handler.go` must return 0.**

- [ ] **Step 10: Update all four implementers' test doubles**

`services/project/service_test.go` — change the fn-field types and methods, and **add a `listPhasesFn` hook** (there is none today):

```go
	listPhasesFn        func(ctx context.Context, tenantID, prodID uuid.UUID, limit, offset int) ([]Phase, error)
	getPhaseFn          func(ctx context.Context, tenantID, id uuid.UUID) (*Phase, error)
	updatePhaseStatusFn func(ctx context.Context, tenantID, id uuid.UUID, status string) error
```

```go
func (m *mockRepo) ListPhases(ctx context.Context, tenantID, prodID uuid.UUID, limit, offset int) ([]Phase, error) {
	if m.listPhasesFn != nil {
		return m.listPhasesFn(ctx, tenantID, prodID, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*Phase, error) {
	if m.getPhaseFn != nil {
		return m.getPhaseFn(ctx, tenantID, id)
	}
	return &Phase{ID: id, TenantID: tenantID, PhaseType: "development", Status: "active"}, nil
}
func (m *mockRepo) UpdatePhaseStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	if m.updatePhaseStatusFn != nil {
		return m.updatePhaseStatusFn(ctx, tenantID, id, status)
	}
	return nil
}
```

`tests/integration/vertical/mocks_test.go` — lines 25-27 and 36-38:

```go
func (m *projectMock) ListPhases(ctx context.Context, tid, pid uuid.UUID, limit, offset int) ([]project.Phase, error) { return nil, nil }
func (m *projectMock) GetPhase(ctx context.Context, tid, id uuid.UUID) (*project.Phase, error) { return nil, nil }
func (m *projectMock) UpdatePhaseStatus(ctx context.Context, tid, id uuid.UUID, status string) error { return nil }
```

```go
func (m *phaseReturnMock) GetPhase(ctx context.Context, tid, id uuid.UUID) (*project.Phase, error) {
	return &project.Phase{ID: id, TenantID: tid, PhaseType: m.phaseType, Status: "active"}, nil
}
```

- [ ] **Step 11: Repair the four flipped tests**

Adding the tenant block to `UpdatePhaseStatus` means tests passing `callerCtx()` (a caller but no tenant) now get `Unauthenticated`. **Exactly four tests flip**, all in `services/project/handler_test.go`:

| test | repair |
|---|---|
| `TestHandler_UpdatePhaseStatus_Success` | `callerCtx()` → `ctxWithTenant(uuid.New())` |
| `TestHandler_UpdatePhaseStatus_InvalidID` | `callerCtx()` → `ctxWithTenant(uuid.New())` |
| `TestHandler_RemoveCrewMember_Success` | leave — Task 2 handles crew |
| `TestHandler_RemoveCrewMember_InvalidID` | leave — Task 2 handles crew |

So **two** repairs in this task. Change the context only. **Do not weaken any assertion** — `_InvalidID` must still assert `InvalidArgument`, which it reaches once a tenant is present.

**If more or fewer than two tests fail, STOP and report.** A surprise means this reading was wrong; slice C predicted 3 flips and got 5, and the discrepancy was only known to be benign because it was investigated.

- [ ] **Step 12: Run the full check**

```bash
go vet ./...
go test ./services/project/ -race -cover
grep -c 'h\.svc\.repo\.' services/project/handler.go   # must be 0
git diff --stat gen/                                    # must be empty
```

Expected: vet clean, all tests pass, coverage ≥ the Step 1 baseline.

- [ ] **Step 13: Verify the tests have teeth**

Follow the predicate-revert procedure in Global Constraints, using `GetPhase`:

```bash
# remove ONLY "AND tenant_id = $2" from the GetPhase query in queries.sql, then:
make generate-sqlc
go test ./services/project/ -run PassesCallerTenant -v
```

Expected: `TestHandler_UpdatePhaseStatus_PassesCallerTenantToRepo` still passes, because the mock — not Postgres — answers in a handler test. **This is the expected result and it means the handler tests alone do not prove the SQL predicate.** Record that in the report, restore the file, and note that the SQL predicate's real proof is CI's real-Postgres integration job.

```bash
git checkout services/project/db/queries.sql && make generate-sqlc && git diff --stat
```

- [ ] **Step 14: Commit**

```bash
git add services/project tests/integration/vertical/mocks_test.go
git commit -m "fix(project): scope phase queries to the caller's tenant (#157)

ListPhases, GetPhase and UpdatePhaseStatus selected and mutated rows by
a caller-supplied UUID with no tenant_id predicate, so any authenticated
user holding production:read in their own tenant could read, and via
UpdatePhaseStatus write, another tenant's phases.

Threads tenantID through Repository, Service and the SQL, matching
GetProduction which already had that shape. The phases table already
declares tenant_id NOT NULL, so no migration is needed.

Also routes the handler through Service.GetPhase/ListPhases instead of
reaching h.svc.repo.* directly -- project was the only handler in the
tree that bypassed its Service, and those call sites had to change
anyway.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: project — crew queries

**Files:**
- Modify: `services/project/db/queries.sql` (ListCrewMembers, RemoveCrewMember)
- Modify: `services/project/db/queries.sql.go` (regenerated)
- Modify: `services/project/db/postgres.go:198-225`
- Modify: `services/project/repository.go:26-27`
- Modify: `services/project/service.go:140-150`
- Modify: `services/project/handler.go:319-357`
- Modify: `services/project/service_test.go` (mockRepo)
- Modify: `tests/integration/vertical/mocks_test.go:29-30`
- Test: `services/project/handler_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 (different methods on the same interface).
- Produces: `Repository.ListCrewMembers(ctx, tenantID, productionID uuid.UUID, limit, offset int) ([]CrewMember, error)`, `Repository.RemoveCrewMember(ctx, tenantID, id uuid.UUID) error`, `Service.ListCrewMembers(ctx, tenantID, productionID uuid.UUID, limit, offset int) ([]CrewMember, error)`, `Service.RemoveCrewMember(ctx, tenantID, id uuid.UUID) error`

- [ ] **Step 1: Write the failing cross-tenant tests**

Add to `services/project/handler_test.go`. `RemoveCrewMember` is a **cross-tenant DELETE** — the sharpest defect in this plan — so its test asserts the repository received the caller's tenant.

```go
// crewRecordingRepo records the tenant ID each crew query receives.
type crewRecordingRepo struct {
	mockRepo
	gotListTenant   uuid.UUID
	gotRemoveTenant uuid.UUID
	removeCalled    bool
}

func (r *crewRecordingRepo) ListCrewMembers(ctx context.Context, tenantID, prodID uuid.UUID, limit, offset int) ([]CrewMember, error) {
	r.gotListTenant = tenantID
	return nil, nil
}

func (r *crewRecordingRepo) RemoveCrewMember(ctx context.Context, tenantID, id uuid.UUID) error {
	r.gotRemoveTenant = tenantID
	r.removeCalled = true
	return nil
}

func TestHandler_ListCrewMembers_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &crewRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.ListCrewMembers(ctxWithTenant(callerTenant), &projectv1.ListCrewMembersRequest{
		ProductionId: uuid.New().String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotListTenant,
		"ListCrewMembers must query with the caller's tenant")
}

func TestHandler_RemoveCrewMember_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &crewRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.RemoveCrewMember(ctxWithTenant(callerTenant), &projectv1.RemoveCrewMemberRequest{
		Id: uuid.New().String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotRemoveTenant,
		"RemoveCrewMember is a DELETE: it must be scoped to the caller's tenant")
}

func TestHandler_RemoveCrewMember_NoTenantDoesNotDelete(t *testing.T) {
	t.Parallel()
	repo := &crewRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	// callerCtx() carries a caller but NO tenant.
	_, err := h.RemoveCrewMember(callerCtx(), &projectv1.RemoveCrewMemberRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.False(t, repo.removeCalled,
		"the DELETE must not be reached without a tenant")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./services/project/ -run 'CrewMember.*PassesCallerTenant|NoTenantDoesNotDelete' -v`
Expected: compile failure — `crewRecordingRepo` does not satisfy `Repository`, because the interface still declares the old arity.

- [ ] **Step 3: Add the tenant predicate to both queries**

In `services/project/db/queries.sql`:

```sql
-- name: ListCrewMembers :many
SELECT * FROM crew_members WHERE production_id = $1 AND tenant_id = $2 ORDER BY created_at ASC LIMIT $3 OFFSET $4;

-- name: RemoveCrewMember :exec
DELETE FROM crew_members WHERE id = $1 AND tenant_id = $2;
```

Read the existing queries first and preserve their exact ordering and column list — only the `WHERE` clause changes.

**Scoping the `RemoveCrewMember` query here does not by itself fix anything**, because `Postgres.RemoveCrewMember` does not call it — it runs its own raw statement (Step 5). The generated query is scoped anyway so the two do not disagree, but the fix that matters is in Step 5.

- [ ] **Step 4: Regenerate sqlc**

```bash
make generate-sqlc
git diff --stat services/project/db/queries.sql.go
```

- [ ] **Step 5: Update the Postgres implementation**

In `services/project/db/postgres.go`:

```go
func (p *Postgres) ListCrewMembers(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]project.CrewMember, error) {
	rows, err := p.q.ListCrewMembers(ctx, ListCrewMembersParams{
		ProductionID: productionID,
		TenantID:     tenantID,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("project: list crew members: %w", err)
	}
	result := make([]project.CrewMember, len(rows))
	for i, row := range rows {
		result[i] = *crewFromDB(row)
	}
	return result, nil
}
```

`RemoveCrewMember` is a **different case: it does not use the generated query at all.** It runs raw inline SQL and ignores the `RemoveCrewMember` in `queries.sql`, which is the same bypass pattern as inventory's `ListCheckouts`. Editing `queries.sql` alone would therefore change nothing. Replace the body:

```go
// RemoveCrewMember deletes a crew member and returns ErrCrewNotFound if the row
// does not exist in the caller's tenant.
func (p *Postgres) RemoveCrewMember(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, "DELETE FROM crew_members WHERE id = $1 AND tenant_id = $2", id, tenantID)
	if err != nil {
		return fmt.Errorf("project: remove crew member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return project.ErrCrewNotFound
	}
	return nil
}
```

The `RowsAffected() == 0` check is what makes a cross-tenant delete return `ErrCrewNotFound` rather than silently succeeding having affected nothing. **Preserve it.**

(The raw statement is kept rather than switched to sqlc because it is already parameterised and the row-count check has no sqlc equivalent here. It is left as the one raw statement in this service; note it in the task report.)

- [ ] **Step 6: Widen the Repository interface**

In `services/project/repository.go`, replace lines 26-27:

```go
	ListCrewMembers(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]CrewMember, error)
	RemoveCrewMember(ctx context.Context, tenantID, id uuid.UUID) error
```

- [ ] **Step 7: Update the Service**

In `services/project/service.go`, replace the two methods at ~140-150:

```go
// ListCrewMembers returns a production's crew, scoped to the caller's tenant.
func (s *Service) ListCrewMembers(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]CrewMember, error) {
	return s.repo.ListCrewMembers(ctx, tenantID, productionID, limit, offset)
}

// RemoveCrewMember removes a crew member, scoped to the caller's tenant.
func (s *Service) RemoveCrewMember(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.RemoveCrewMember(ctx, tenantID, id)
}
```

- [ ] **Step 8: Update the handler**

In `services/project/handler.go`, `ListCrewMembers` gains the tenant block and passes it down:

```go
func (h *Handler) ListCrewMembers(ctx context.Context, req *projectv1.ListCrewMembersRequest) (*projectv1.ListCrewMembersResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production ID")
	}

	// Proto does not carry limit/offset yet; apply a server-side default.
	// Update the proto and pass through req fields when pagination is added.
	const defaultLimit = 50
	members, err := h.svc.ListCrewMembers(ctx, tenantID, productionID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*projectv1.CrewMember, len(members))
	for i := range members {
		out[i] = crewToProto(&members[i])
	}
	return &projectv1.ListCrewMembersResponse{Members: out}, nil
}
```

`RemoveCrewMember` (line ~344) — note it gates on `"resource:manage"`, **not** `production:write`; leave that string exactly as it is:

```go
func (h *Handler) RemoveCrewMember(ctx context.Context, req *projectv1.RemoveCrewMemberRequest) (*projectv1.RemoveCrewMemberResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "resource:manage"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid crew member ID")
	}

	if err := h.svc.RemoveCrewMember(ctx, tenantID, id); err != nil {
		return nil, grpcErr(err)
	}
	return &projectv1.RemoveCrewMemberResponse{}, nil
}
```

- [ ] **Step 9: Update the test doubles**

`services/project/service_test.go` — change the `listCrewFn` signature and add a `removeCrewFn` hook (there is none today):

```go
	listCrewFn   func(ctx context.Context, tenantID, prodID uuid.UUID) ([]CrewMember, error)
	removeCrewFn func(ctx context.Context, tenantID, id uuid.UUID) error
```

```go
func (m *mockRepo) ListCrewMembers(ctx context.Context, tenantID, prodID uuid.UUID, limit, offset int) ([]CrewMember, error) {
	if m.listCrewFn != nil {
		return m.listCrewFn(ctx, tenantID, prodID)
	}
	return nil, nil
}
func (m *mockRepo) RemoveCrewMember(ctx context.Context, tenantID, id uuid.UUID) error {
	if m.removeCrewFn != nil {
		return m.removeCrewFn(ctx, tenantID, id)
	}
	return nil
}
```

`tests/integration/vertical/mocks_test.go` — lines 29-30:

```go
func (m *projectMock) ListCrewMembers(ctx context.Context, tid, pid uuid.UUID, limit, offset int) ([]project.CrewMember, error) { return nil, nil }
func (m *projectMock) RemoveCrewMember(ctx context.Context, tid, id uuid.UUID) error { return nil }
```

- [ ] **Step 10: Repair the two flipped tests**

`TestHandler_RemoveCrewMember_Success` and `TestHandler_RemoveCrewMember_InvalidID` use `callerCtx()` and now get `Unauthenticated`. Change to `ctxWithTenant(uuid.New())`. **Do not weaken assertions** — `_InvalidID` must still assert `InvalidArgument`.

**Exactly two tests flip. If more or fewer, STOP and report.**

- [ ] **Step 11: Run the full check**

```bash
go vet ./...
go test ./services/project/ -race -cover
git diff --stat gen/   # must be empty
```

Expected: vet clean, tests pass, coverage ≥ Task 1's figure.

- [ ] **Step 12: Commit**

```bash
git add services/project tests/integration/vertical/mocks_test.go
git commit -m "fix(project): scope crew queries to the caller's tenant (#157)

ListCrewMembers read and RemoveCrewMember DELETED rows by a
caller-supplied UUID with no tenant_id predicate. RemoveCrewMember is a
cross-tenant delete and is not named in #157's original text -- it was
found by scanning every queries.sql for missing tenant predicates.

Threads tenantID through Repository, Service and the SQL. The
crew_members table already declares tenant_id NOT NULL, so no migration
is needed.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: budget — line-item queries

Covers five defects: two unscoped reads, one tautological write, one latent query, and the second-order recompute.

**Files:**
- Modify: `services/budget/db/queries.sql` (GetLineItem, ListLineItems, LockLineItem, UpdateBudgetTotals)
- Modify: `services/budget/db/queries.sql.go` (regenerated)
- Modify: `services/budget/db/postgres.go:120-200` (GetLineItem, ListLineItems, UpdateLineItemActuals, CheckLineAvailability, CreateLineItem's UpdateBudgetTotals call)
- Modify: `services/budget/repository.go:20-23`
- Modify: `services/budget/service.go:114-135`
- Modify: `services/budget/handler.go:262, 269-310, 325-360`
- Modify: `services/budget/service_test.go` (mockRepo)
- Modify: `tests/integration/vertical/mocks_test.go:49-52`
- Test: `services/budget/handler_test.go`

**Interfaces:**
- Consumes: nothing from Tasks 1-2.
- Produces: `Repository.GetLineItem(ctx, tenantID, id uuid.UUID) (*BudgetLineItem, error)`, `Repository.ListLineItems(ctx, tenantID, budgetID uuid.UUID, limit, offset int) ([]BudgetLineItem, error)`, `Repository.UpdateLineItemActuals(ctx, tenantID, id uuid.UUID, actualAmount, committedAmount decimal.Decimal) error`, `Repository.CheckLineAvailability(ctx, tenantID, id uuid.UUID) (decimal.Decimal, error)`, and the matching `Service` methods with the same signatures.

- [ ] **Step 1: Write the failing cross-tenant tests**

Add to `services/budget/handler_test.go`. Note `UpdateLineItemActuals` writes monetary amounts, so its cross-tenant case is a financial-integrity defect.

```go
// lineItemRecordingRepo records the tenant ID each line-item query receives.
// budget's mockRepo returns a usable BudgetLineItem by default, so asserting
// only on the status code would pass against unscoped code.
type lineItemRecordingRepo struct {
	mockRepo
	gotGetTenant    uuid.UUID
	gotListTenant   uuid.UUID
	gotUpdateTenant uuid.UUID
	updateCalled    bool
}

func (r *lineItemRecordingRepo) GetLineItem(ctx context.Context, tenantID, id uuid.UUID) (*BudgetLineItem, error) {
	r.gotGetTenant = tenantID
	return &BudgetLineItem{ID: id, TenantID: tenantID, CategoryID: "above_the_line"}, nil
}

func (r *lineItemRecordingRepo) ListLineItems(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]BudgetLineItem, error) {
	r.gotListTenant = tenantID
	return nil, nil
}

func (r *lineItemRecordingRepo) UpdateLineItemActuals(ctx context.Context, tenantID, id uuid.UUID, actual, committed decimal.Decimal) error {
	r.gotUpdateTenant = tenantID
	r.updateCalled = true
	return nil
}

func TestHandler_GetLineItem_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &lineItemRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.GetLineItem(ctxWithTenant(callerTenant), &budgetv1.GetLineItemRequest{
		Id: uuid.New().String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotGetTenant,
		"GetLineItem must query with the caller's tenant")
}

func TestHandler_ListLineItems_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &lineItemRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.ListLineItems(ctxWithTenant(callerTenant), &budgetv1.ListLineItemsRequest{
		BudgetId: uuid.New().String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotListTenant,
		"ListLineItems must query with the caller's tenant")
}

func TestHandler_UpdateLineItemActuals_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &lineItemRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.UpdateLineItemActuals(ctxWithTenant(callerTenant), &budgetv1.UpdateLineItemActualsRequest{
		Id:              uuid.New().String(),
		ActualAmount:    "100.00",
		CommittedAmount: "50.00",
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotUpdateTenant,
		"UpdateLineItemActuals writes money: it must be scoped to the caller's tenant")
}

func TestHandler_UpdateLineItemActuals_NoTenantDoesNotWrite(t *testing.T) {
	t.Parallel()
	repo := &lineItemRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	// callerCtx() carries a caller but NO tenant.
	_, err := h.UpdateLineItemActuals(callerCtx(), &budgetv1.UpdateLineItemActualsRequest{
		Id:              uuid.New().String(),
		ActualAmount:    "100.00",
		CommittedAmount: "50.00",
	})

	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.False(t, repo.updateCalled,
		"the monetary write must not be reached without a tenant")
}
```

Check the exact request field names in `gen/budget/v1/` before writing — `UpdateLineItemActualsRequest` may name its id field differently.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./services/budget/ -run 'PassesCallerTenant|NoTenantDoesNotWrite' -v`
Expected: compile failure — `lineItemRecordingRepo` does not satisfy `Repository`.

- [ ] **Step 3: Add tenant predicates to the four queries**

In `services/budget/db/queries.sql`:

```sql
-- name: UpdateBudgetTotals :exec
UPDATE budgets
SET total_amount = (SELECT COALESCE(SUM(budgeted_amount), 0) FROM budget_line_items WHERE budget_id = $1 AND tenant_id = $2),
    updated_at  = now()
WHERE id = $1 AND tenant_id = $2;

-- name: GetLineItem :one
SELECT * FROM budget_line_items WHERE id = $1 AND tenant_id = $2;

-- name: ListLineItems :many
SELECT * FROM budget_line_items WHERE budget_id = $1 AND tenant_id = $2 ORDER BY category_id, created_at ASC LIMIT $3 OFFSET $4;

-- name: LockLineItem :exec
UPDATE budget_line_items SET is_locked = true, updated_at = now()
WHERE id = $1 AND tenant_id = $2;
```

`UpdateLineItemAmounts` already carries a `tenant_id` predicate — leave its SQL alone. What changes is where its tenant comes from (Step 5).

- [ ] **Step 4: Regenerate sqlc**

```bash
make generate-sqlc
git diff --stat services/budget/db/queries.sql.go
```

- [ ] **Step 5: Fix the Postgres implementation, including the tautological lookup**

In `services/budget/db/postgres.go`:

```go
func (p *Postgres) GetLineItem(ctx context.Context, tenantID, id uuid.UUID) (*budget.BudgetLineItem, error) {
	row, err := p.q.GetLineItem(ctx, GetLineItemParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, budget.ErrLineItemNotFound
		}
		return nil, fmt.Errorf("budget: get line item: %w", err)
	}
	return lineItemFromDB(row), nil
}

func (p *Postgres) ListLineItems(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]budget.BudgetLineItem, error) {
	rows, err := p.q.ListLineItems(ctx, ListLineItemsParams{
		BudgetID: budgetID,
		TenantID: tenantID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("budget: list line items: %w", err)
	}
	result := make([]budget.BudgetLineItem, len(rows))
	for i, row := range rows {
		result[i] = *lineItemFromDB(row)
	}
	return result, nil
}

func (p *Postgres) CheckLineAvailability(ctx context.Context, tenantID, id uuid.UUID) (decimal.Decimal, error) {
	li, err := p.GetLineItem(ctx, tenantID, id)
	if err != nil {
		return decimal.Zero, err
	}
	available := li.BudgetedAmount.Sub(li.ActualAmount).Sub(li.CommittedAmount)
	return available, nil
}
```

`UpdateLineItemActuals` — **delete the resolve-from-row lookup entirely.** It currently reads:

```go
// Resolve tenant_id for the WHERE clause.
row := p.db.QueryRow(ctx, "SELECT tenant_id FROM budget_line_items WHERE id = $1", id)
```

That makes the `AND tenant_id = $2` predicate a tautology — `$2` comes from the target row, so it can never fail. Replace with the caller's tenant:

```go
func (p *Postgres) UpdateLineItemActuals(ctx context.Context, tenantID, id uuid.UUID, actualAmount, committedAmount decimal.Decimal) error {
	li, err := p.q.UpdateLineItemAmounts(ctx, UpdateLineItemAmountsParams{
		ID:              id,
		TenantID:        tenantID,
		BudgetedAmount:  pgtype.Numeric{Valid: false}, // COALESCE(NULL, budgeted_amount) — keep existing
		ActualAmount:    pgNumericFromDecimal(actualAmount),
		CommittedAmount: pgNumericFromDecimal(committedAmount),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return budget.ErrLineItemNotFound
		}
		return fmt.Errorf("budget: update line item actuals: %w", err)
	}

	// Keep budget total in sync.
	if err := p.q.UpdateBudgetTotals(ctx, UpdateBudgetTotalsParams{BudgetID: li.BudgetID, TenantID: tenantID}); err != nil {
		return fmt.Errorf("budget: update budget totals after actuals: %w", err)
	}
	return nil
}
```

`CreateLineItem` also calls `UpdateBudgetTotals` — it has `li.TenantID` on the struct it just inserted, so pass that. Read the current code and use the field that is already in scope.

Match the generated param struct field names exactly; read `queries.sql.go` after Step 4.

- [ ] **Step 6: Widen the Repository interface**

In `services/budget/repository.go`, replace lines 20-23:

```go
	GetLineItem(ctx context.Context, tenantID, id uuid.UUID) (*BudgetLineItem, error)
	ListLineItems(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]BudgetLineItem, error)
	UpdateLineItemActuals(ctx context.Context, tenantID, id uuid.UUID, actualAmount, committedAmount decimal.Decimal) error
	CheckLineAvailability(ctx context.Context, tenantID, id uuid.UUID) (decimal.Decimal, error)
```

- [ ] **Step 7: Update the Service**

In `services/budget/service.go`, replace the four pass-throughs at ~114-135:

```go
// GetLineItem returns a line item, scoped to the caller's tenant.
func (s *Service) GetLineItem(ctx context.Context, tenantID, id uuid.UUID) (*BudgetLineItem, error) {
	return s.repo.GetLineItem(ctx, tenantID, id)
}

// ListLineItems lists line items for a budget, capped at 200 per page.
func (s *Service) ListLineItems(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]BudgetLineItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.repo.ListLineItems(ctx, tenantID, budgetID, limit, offset)
}

// UpdateLineItemActuals updates actual and committed amounts on a line item.
func (s *Service) UpdateLineItemActuals(ctx context.Context, tenantID, id uuid.UUID, actualAmount, committedAmount decimal.Decimal) error {
	return s.repo.UpdateLineItemActuals(ctx, tenantID, id, actualAmount, committedAmount)
}

// CheckLineAvailability returns the available amount on a line item
// (budgeted - actual - committed).
func (s *Service) CheckLineAvailability(ctx context.Context, tenantID, id uuid.UUID) (decimal.Decimal, error) {
	return s.repo.CheckLineAvailability(ctx, tenantID, id)
}
```

- [ ] **Step 8: Update the handler**

In `services/budget/handler.go`, four RPCs need the tenant block added above their existing gate, then the tenant passed to the service: `GetLineItem` (~269), `ListLineItems` (~286), `UpdateLineItemActuals` (~325), `CheckLineAvailability` (~343). The block is identical in each:

```go
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
```

It goes **above** the existing `interceptor.RequirePermission` call, matching `CreateBudget` and every other tenant-aware RPC in the file. Then each service call gains `tenantID` as its first id argument, e.g.:

```go
	li, err := h.svc.GetLineItem(ctx, tenantID, id)
	items, err := h.svc.ListLineItems(ctx, tenantID, budgetID, defaultLimit, 0)
	available, err := h.svc.CheckLineAvailability(ctx, tenantID, id)
```

`CreateLineItem` at line ~262 reads back with `h.svc.GetLineItem(ctx, li.ID)` — it already has a tenant in scope from its own block; pass it.

- [ ] **Step 9: Update the test doubles**

`services/budget/service_test.go` — four fn-fields and methods:

```go
	getLineItemFn           func(ctx context.Context, tenantID, id uuid.UUID) (*BudgetLineItem, error)
	listLineItemsFn         func(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]BudgetLineItem, error)
	updateLineItemActualsFn func(ctx context.Context, tenantID, id uuid.UUID, actual, committed decimal.Decimal) error
	checkLineAvailabilityFn func(ctx context.Context, tenantID, id uuid.UUID) (decimal.Decimal, error)
```

```go
func (m *mockRepo) GetLineItem(ctx context.Context, tenantID, id uuid.UUID) (*BudgetLineItem, error) {
	if m.getLineItemFn != nil {
		return m.getLineItemFn(ctx, tenantID, id)
	}
	return &BudgetLineItem{ID: id, TenantID: tenantID, CategoryID: "above_the_line"}, nil
}
func (m *mockRepo) ListLineItems(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]BudgetLineItem, error) {
	if m.listLineItemsFn != nil {
		return m.listLineItemsFn(ctx, tenantID, budgetID, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateLineItemActuals(ctx context.Context, tenantID, id uuid.UUID, actual, committed decimal.Decimal) error {
	if m.updateLineItemActualsFn != nil {
		return m.updateLineItemActualsFn(ctx, tenantID, id, actual, committed)
	}
	return nil
}
func (m *mockRepo) CheckLineAvailability(ctx context.Context, tenantID, id uuid.UUID) (decimal.Decimal, error) {
	if m.checkLineAvailabilityFn != nil {
		return m.checkLineAvailabilityFn(ctx, tenantID, id)
	}
	return decimal.NewFromInt(0), nil
}
```

`tests/integration/vertical/mocks_test.go` — lines 49-52:

```go
func (m *budgetMock) GetLineItem(ctx context.Context, tid, id uuid.UUID) (*budget.BudgetLineItem, error) { return nil, nil }
func (m *budgetMock) ListLineItems(ctx context.Context, tid, budgetID uuid.UUID, limit, offset int) ([]budget.BudgetLineItem, error) { return nil, nil }
func (m *budgetMock) UpdateLineItemActuals(ctx context.Context, tid, id uuid.UUID, actual, committed decimal.Decimal) error { return nil }
func (m *budgetMock) CheckLineAvailability(ctx context.Context, tid, id uuid.UUID) (decimal.Decimal, error) { return decimal.NewFromInt(1000000), nil }
```

- [ ] **Step 10: Repair the flipped tests**

Fourteen tests in `services/budget/handler_test.go` use `callerCtx()` and now get `Unauthenticated`:

`TestHandler_GetLineItem_Success`, `_InvalidID`, `_NotFound`, `_Denied`; `TestHandler_ListLineItems_Success`, `_InvalidBudgetID`, `_Denied`; `TestHandler_UpdateLineItemActuals_Success`, `_InvalidID`, `_InvalidActual`, `_InvalidCommitted`; `TestHandler_CheckLineAvailability_Success`, `_InvalidID`, `_Denied`.

Repair each by replacing `callerCtx()` with `ctxWithTenant(uuid.New())`. **Do not weaken any assertion.** The `_Denied` tests must still assert `PermissionDenied` — they reach the gate once a tenant is present, because the tenant block sits above it.

**Exactly fourteen tests flip. If the count differs, STOP and report** — a difference means this reading was wrong.

- [ ] **Step 11: Run the full check**

```bash
go vet ./...
go test ./services/budget/ -race -cover
grep -c 'SELECT tenant_id FROM budget_line_items' services/budget/db/postgres.go   # must be 0
git diff --stat gen/   # must be empty
```

Expected: vet clean, tests pass, coverage ≥ baseline, the tautological lookup gone.

- [ ] **Step 12: Commit**

```bash
git add services/budget tests/integration/vertical/mocks_test.go
git commit -m "fix(budget): scope line-item queries to the caller's tenant (#157)

GetLineItem and ListLineItems had no tenant_id predicate, so any
authenticated user holding budget:read in their own tenant could read
another tenant's line items -- the RPCs #139 slice C gated, confirming
a permission gate is necessary but not sufficient.

UpdateLineItemActuals was worse: its SQL carried AND tenant_id = \$2,
but Go supplied \$2 by reading it off the target row first, making the
predicate a tautology. That is a cross-tenant write of NUMERIC(14,2)
monetary amounts. The lookup is deleted and the caller's tenant used.

Also scopes LockLineItem (no callers today, fixed so the first caller
does not inherit the defect) and UpdateBudgetTotals.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: inventory — checkout queries

Covers four defects: two raw-SQL reads that bypass `queries.sql`, one tautological write, and one latent query.

**Files:**
- Modify: `services/inventory/db/queries.sql` (GetActiveCheckout; add a GetCheckout query)
- Modify: `services/inventory/db/queries.sql.go` (regenerated)
- Modify: `services/inventory/db/postgres.go:115-135` (CheckInAsset), `:137-165` (GetCheckout), `:167-197` (ListCheckouts), `:199-210` (delete resolveTenantForCheckout)
- Modify: `services/inventory/repository.go:17-20`
- Modify: `services/inventory/service.go:73-78` and the `GetCheckout`/`ListCheckouts` methods
- Modify: `services/inventory/handler.go` (CheckInAsset, ListCheckouts)
- Modify: `services/inventory/service_test.go` (mockRepo)
- Modify: `tests/integration/vertical/mocks_test.go:80-82`
- Test: `services/inventory/handler_test.go`

**Interfaces:**
- Consumes: nothing from Tasks 1-3.
- Produces: `Repository.CheckInAsset(ctx, tenantID, checkoutID uuid.UUID, conditionIn string) error`, `Repository.GetCheckout(ctx, tenantID, id uuid.UUID) (*AssetCheckout, error)`, `Repository.ListCheckouts(ctx, tenantID, assetID uuid.UUID) ([]AssetCheckout, error)`, and matching `Service` methods.

**⚠️ Do NOT add a permission gate to `ListCheckouts` in this task.** It currently has neither a tenant check nor a gate. Add the tenant check only. Gating it on `inventory:read` would lock out every role that can check an asset out, because that string is granted to `inventory_manager` alone — not even `super_admin`. That grant-matrix fix is #139 slice D.

- [ ] **Step 1: Write the failing cross-tenant tests**

Add to `services/inventory/handler_test.go`. Note this file has `allowAllPerm` and `ctxWithTenant` but **no `denyPerm`** — you do not need one for these tests.

```go
// checkoutRecordingRepo records the tenant ID each checkout query receives.
// inventory's mockRepo returns a usable AssetCheckout by default, so a
// status-code assertion alone would pass against unscoped code.
type checkoutRecordingRepo struct {
	mockRepo
	gotListTenant    uuid.UUID
	gotGetTenant     uuid.UUID
	gotCheckInTenant uuid.UUID
}

func (r *checkoutRecordingRepo) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID) ([]AssetCheckout, error) {
	r.gotListTenant = tenantID
	return nil, nil
}

func (r *checkoutRecordingRepo) GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*AssetCheckout, error) {
	r.gotGetTenant = tenantID
	return &AssetCheckout{ID: id, TenantID: tenantID}, nil
}

func (r *checkoutRecordingRepo) CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, conditionIn string) error {
	r.gotCheckInTenant = tenantID
	return nil
}

func TestHandler_ListCheckouts_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &checkoutRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.ListCheckouts(ctxWithTenant(callerTenant), &inventoryv1.ListCheckoutsRequest{
		AssetId: uuid.New().String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotListTenant,
		"ListCheckouts must query with the caller's tenant")
}

func TestHandler_CheckInAsset_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	repo := &checkoutRecordingRepo{}
	h := NewHandler(NewService(repo)).WithPermissionChecker(allowAllPerm{})

	_, err := h.CheckInAsset(ctxWithTenant(callerTenant), &inventoryv1.CheckInAssetRequest{
		CheckoutId:  uuid.New().String(),
		AssetId:     uuid.New().String(),
		ConditionIn: "good",
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, repo.gotCheckInTenant,
		"CheckInAsset must write with the caller's tenant, which Service already held")
	require.Equal(t, callerTenant, repo.gotGetTenant,
		"the read-back after check-in must also be tenant-scoped")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./services/inventory/ -run PassesCallerTenant -v`
Expected: compile failure — `checkoutRecordingRepo` does not satisfy `Repository`.

- [ ] **Step 3: Add a GetCheckout query and scope GetActiveCheckout**

In `services/inventory/db/queries.sql`, scope the latent query and add one for `GetCheckout` (which is currently raw SQL in Go):

```sql
-- name: GetActiveCheckout :one
SELECT * FROM asset_checkouts
WHERE asset_id = $1 AND tenant_id = $2 AND checked_in_at IS NULL
LIMIT 1;

-- name: GetCheckout :one
SELECT * FROM asset_checkouts WHERE id = $1 AND tenant_id = $2;
```

**Leave the existing `ListCheckouts` query as it is** — it already reads `WHERE tenant_id = $1 AND ($2::uuid IS NULL OR asset_id = $2) AND ($3::uuid IS NULL OR production_id = $3)`. It is correct and simply unused; Step 5 wires it up.

- [ ] **Step 4: Regenerate sqlc**

```bash
make generate-sqlc
git diff --stat services/inventory/db/queries.sql.go
```

- [ ] **Step 5: Replace the raw SQL with the generated queries**

In `services/inventory/db/postgres.go`, replace the two hand-written implementations. `GetCheckout` currently builds a `const sql = ...` string and scans ten columns by hand; replace the whole body:

```go
func (p *Postgres) GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*inventory.AssetCheckout, error) {
	row, err := p.q.GetCheckout(ctx, GetCheckoutParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, inventory.ErrAssetNotFound
		}
		return nil, fmt.Errorf("inventory: get checkout: %w", err)
	}
	return checkoutFromDB(row), nil
}
```

`ListCheckouts` — use the existing generated query, which already filters tenant. Read `ListCheckoutsParams` in `queries.sql.go` for the exact field names and nullable types (`asset_id` and `production_id` are optional `::uuid` params):

```go
func (p *Postgres) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID) ([]inventory.AssetCheckout, error) {
	rows, err := p.q.ListCheckouts(ctx, ListCheckoutsParams{
		TenantID: tenantID,
		AssetID:  assetID, // match the generated nullable type
	})
	if err != nil {
		return nil, fmt.Errorf("inventory: list checkouts: %w", err)
	}
	result := make([]inventory.AssetCheckout, len(rows))
	for i, row := range rows {
		result[i] = *checkoutFromDB(row)
	}
	return result, nil
}
```

`CheckInAsset` — **delete the `resolveTenantForCheckout` call and the helper itself** (`postgres.go:199-210`), and use the caller's tenant:

```go
func (p *Postgres) CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, conditionIn string) error {
	_, err := p.q.CheckinAsset(ctx, CheckinAssetParams{
		ID:          checkoutID,
		TenantID:    tenantID,
		ConditionIn: pgTextFromString(conditionIn),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return inventory.ErrAssetNotFound
		}
		return fmt.Errorf("inventory: check in asset: %w", err)
	}
	return nil
}
```

Read the current body first and preserve any behaviour after the query that this snippet omits.

- [ ] **Step 6: Widen the Repository interface**

In `services/inventory/repository.go`, replace lines 17-20:

```go
	// Checkouts
	CheckOutAsset(ctx context.Context, c *AssetCheckout) error
	CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, conditionIn string) error
	GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*AssetCheckout, error)
	ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID) ([]AssetCheckout, error)
```

- [ ] **Step 7: Update the Service — it already holds the tenant**

`Service.CheckInAsset` already takes `tenantID` and passes it to `UpdateAssetStatus` on the next line while dropping it for the checkout row. Fix that, and thread the tenant through the two read methods:

```go
func (s *Service) CheckInAsset(ctx context.Context, tenantID, checkoutID, assetID uuid.UUID, conditionIn string) error {
	if err := s.repo.CheckInAsset(ctx, tenantID, checkoutID, conditionIn); err != nil {
		return err
	}
	return s.repo.UpdateAssetStatus(ctx, tenantID, assetID, "available")
}
```

Read the existing `Service.GetCheckout` and `Service.ListCheckouts` and add `tenantID` as the first id parameter to each, passing it to the repository.

- [ ] **Step 8: Update the handler**

`CheckInAsset` already has `tenantID` in scope from its own tenant block — pass it to the read-back:

```go
	out, err := h.svc.GetCheckout(ctx, tenantID, checkoutID)
```

`ListCheckouts` has **no tenant block at all**. Add one, and pass the tenant down. **Add no permission gate** — see the warning at the top of this task:

```go
func (h *Handler) ListCheckouts(ctx context.Context, req *inventoryv1.ListCheckoutsRequest) (*inventoryv1.ListCheckoutsResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	assetID, err := uuid.Parse(req.GetAssetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid asset_id")
	}

	checkouts, err := h.svc.ListCheckouts(ctx, tenantID, assetID)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*inventoryv1.AssetCheckout, len(checkouts))
	for i := range checkouts {
		out[i] = checkoutToProto(&checkouts[i])
	}
	return &inventoryv1.ListCheckoutsResponse{Checkouts: out}, nil
}
```

Check whether `tenant` is already imported in this file; add the import if not.

- [ ] **Step 9: Update the test doubles**

`services/inventory/service_test.go`:

```go
	checkInAssetFn  func(ctx context.Context, tenantID, checkoutID uuid.UUID, conditionIn string) error
	getCheckoutFn   func(ctx context.Context, tenantID, id uuid.UUID) (*AssetCheckout, error)
	listCheckoutsFn func(ctx context.Context, tenantID, assetID uuid.UUID) ([]AssetCheckout, error)
```

```go
func (m *mockRepo) CheckInAsset(ctx context.Context, tenantID, checkoutID uuid.UUID, conditionIn string) error {
	if m.checkInAssetFn != nil {
		return m.checkInAssetFn(ctx, tenantID, checkoutID, conditionIn)
	}
	return nil
}
func (m *mockRepo) GetCheckout(ctx context.Context, tenantID, id uuid.UUID) (*AssetCheckout, error) {
	if m.getCheckoutFn != nil {
		return m.getCheckoutFn(ctx, tenantID, id)
	}
	return &AssetCheckout{ID: id, TenantID: tenantID}, nil
}
func (m *mockRepo) ListCheckouts(ctx context.Context, tenantID, assetID uuid.UUID) ([]AssetCheckout, error) {
	if m.listCheckoutsFn != nil {
		return m.listCheckoutsFn(ctx, tenantID, assetID)
	}
	return nil, nil
}
```

`tests/integration/vertical/mocks_test.go` — lines 80-82:

```go
func (m *inventoryMock) CheckInAsset(ctx context.Context, tid, checkoutID uuid.UUID, conditionIn string) error { return nil }
func (m *inventoryMock) GetCheckout(ctx context.Context, tid, id uuid.UUID) (*inventory.AssetCheckout, error) { return &inventory.AssetCheckout{ID: id, TenantID: tid}, nil }
func (m *inventoryMock) ListCheckouts(ctx context.Context, tid, assetID uuid.UUID) ([]inventory.AssetCheckout, error) { return nil, nil }
```

- [ ] **Step 10: Repair flipped tests**

`services/inventory/handler_test.go` has no `callerCtx()` helper, so the flip predictor used in Tasks 1-3 does not apply. Any existing `ListCheckouts` test that passes a bare context now gets `Unauthenticated` from the new tenant block. Run the suite and count:

```bash
go test ./services/inventory/ 2>&1 | grep -E '^\s+--- FAIL' | sort
```

Repair each by supplying `ctxWithTenant(uuid.New())`. **Do not weaken assertions.** Record the count in the task report — there is no prediction to check against here, so state what you found.

- [ ] **Step 11: Run the full check**

```bash
go vet ./...
go test ./services/inventory/ -race -cover
grep -c 'resolveTenantForCheckout' services/inventory/db/postgres.go   # must be 0
grep -c 'const sql' services/inventory/db/postgres.go                  # must be 0
git diff --stat gen/   # must be empty
```

Expected: vet clean, tests pass, coverage ≥ baseline, both raw-SQL constants and the tautological helper gone.

- [ ] **Step 12: Commit**

```bash
git add services/inventory tests/integration/vertical/mocks_test.go
git commit -m "fix(inventory): scope checkout queries to the caller's tenant (#157)

GetCheckout and ListCheckouts were hand-written SQL in Go that selected
the tenant_id column without filtering on it -- invisible to a scan of
queries.sql, and invisible to a grep for tenant_id. A correct,
tenant-filtering ListCheckouts already existed in queries.sql and was
simply unused; this wires it up rather than patching the raw string.

CheckInAsset resolved the tenant from the target row to satisfy its
WHERE clause, making the predicate a tautology. Service.CheckInAsset
already had the caller's tenant and passed it to UpdateAssetStatus on
the very next line while dropping it for the checkout row -- the same
shape as the discarded tenantID #146 fixed in Service.AssignRole.

Also scopes GetActiveCheckout, which has no callers today.

No permission gate is added: inventory:read is granted to
inventory_manager alone, so gating ListCheckouts would lock out every
role that can check an asset out. That is #139 slice D.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Whole-branch verification

Run after all four tasks, before opening the PR.

- [ ] **Step 1: Whole-tree build and vet**

```bash
go vet ./...
go build ./cmd/...
```

- [ ] **Step 2: Full test suite with race detector**

```bash
go test ./... -short
go test ./services/project/ ./services/budget/ ./services/inventory/ -race
```

- [ ] **Step 3: Confirm no defect of either class remains in the three services**

```bash
# no unscoped queries left in the three services
grep -n 'WHERE id = \$1;\|WHERE production_id = \$1 \|WHERE budget_id = \$1 \|WHERE asset_id = \$1 ' \
  services/project/db/queries.sql services/budget/db/queries.sql services/inventory/db/queries.sql

# no tautological tenant lookups left
grep -rn 'SELECT tenant_id FROM' services/project services/budget services/inventory

# no raw SQL left in inventory's repo
grep -c 'const sql' services/inventory/db/postgres.go
```

Expected: first grep returns nothing, second returns nothing, third returns 0.

- [ ] **Step 4: Confirm the generated protobuf did not move**

```bash
git diff --stat gen/          # must be empty
git diff --stat migrations/   # must be empty
```

- [ ] **Step 5: Coverage did not regress**

```bash
go test ./services/project/ ./services/budget/ ./services/inventory/ -cover
```

Compare each against the baseline recorded at the start of its task. All three are in the `others ≥ 75%` tier.

- [ ] **Step 6: Confirm the worktree is clean**

```bash
git worktree list   # only the repo itself and the long-lived thittam-demo-wt
git status --short  # clean
```

- [ ] **Step 7: Push and open the PR**

```bash
git push -u origin fix/tenant-isolation-157
```

PR body must state: closes the tenant-isolation half of #157; 14 defects across three classes; no migration, no proto change, no new permission string; `billing`/`document`/`notifications`/`iam` deliberately excluded and tracked as #159. Flag for senior review — this is a security change touching the tenant boundary and needs 2 approvals.

- [ ] **Step 8: Confirm CI is green**

```bash
gh pr checks <number>
```

**Local green is not CI green.** Do not declare the PR ready until every check passes — in particular the real-Postgres integration job, which is the only thing that actually exercises the SQL predicates these tasks added.
