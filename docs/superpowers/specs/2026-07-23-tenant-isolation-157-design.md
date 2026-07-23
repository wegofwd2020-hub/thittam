# Tenant isolation on child-row queries — project, budget, inventory — design

**Issue:** #157, expanded. **Branch:** `fix/tenant-isolation-157`, base = current `main` (`e1871c5`).
**Follows:** #138 (authentication), #144 (tenant boundary), #146 (role-assignment), #149 (ledger authorization + `ActorFromRequest`), #139 slice A (`ChangePassword`), #139 slice C (read-path gates).
**Relates to:** #139 §3 — "prove the read paths are tenant-isolated". This spec is the discovery that they are not, and the fix.

## 1. The problem

Seven queries across `project` and `budget` select or mutate rows by a caller-supplied UUID with **no `tenant_id` predicate**. Their parent tables scope correctly; the child tables do not:

```sql
-- productions: correct
SELECT * FROM productions WHERE tenant_id = $1 ...

-- phases, crew_members, budget_line_items: no tenant predicate
SELECT * FROM phases WHERE production_id = $1 ORDER BY created_at ASC LIMIT $2 OFFSET $3;
SELECT * FROM phases WHERE id = $1;
UPDATE phases SET status = $2, updated_at = now() WHERE id = $1 RETURNING *;
SELECT * FROM budget_line_items WHERE id = $1;
```

The service layer does not compensate. `Service.GetLineItem(ctx, id)` is `return s.repo.GetLineItem(ctx, id)` — a pure pass-through. `Service.RemoveCrewMember(ctx, id)` likewise. This was checked specifically, because the ledger *does* compensate and the SQL alone cannot distinguish the two cases (§2.4).

So an authenticated user in tenant A who holds `production:read` **in their own tenant** can read tenant B's phases and crew, and — via `UpdatePhaseStatus` and `RemoveCrewMember` — write and delete them.

### 1.1 Why the slice C gates do not close this

`interceptor.RequirePermission` asks whether *the caller* holds a permission. It is answered from the caller's own tenant and never consults the target row. A caller holding `budget:read` legitimately in tenant A passes the gate and then reads tenant B's line item.

`GetLineItem`, `ListLineItems` and `CheckLineAvailability` are exactly the RPCs #139 slice C gated. The gate is correct and necessary; it is not sufficient. Same for project's `ListPhases` and `ListCrewMembers`. **A permission gate and a tenant predicate are orthogonal controls, and slice C only installed the first.**

### 1.2 What limits the impact, and what does not

Exploitation requires the target row's UUID, and v4 UUIDs are not enumerable. This is not a scanning primitive.

It is still a live cross-tenant defect, for two reasons. Resource UUIDs are not secrets — they appear in URLs, CSV exports, logs, support tickets, and are visible to anyone who was ever legitimately a member of the target tenant, including former employees and contractors who worked across productions. And the write paths (`UpdatePhaseStatus`, `RemoveCrewMember`) mean the damage is not bounded by what the attacker can already read: an id learned once, from any source, remains a durable write capability.

## 2. Triage — 14 unscoped queries, 10 defects

A tree-wide scan of every `services/*/db/queries.sql` for queries with no `tenant_id` predicate found 14. They fall into four classes. Only the first three are defects; the fourth is correct and must be left alone.

### 2.1 Live — caller-reachable with an arbitrary UUID (7)

| service | query | reached from | severity |
|---|---|---|---|
| project | `ListPhases` | `ListPhases` RPC | cross-tenant read |
| project | `GetPhase` | `CreatePhase`, `UpdatePhaseStatus` RPCs | cross-tenant read |
| project | `UpdatePhaseStatus` | `UpdatePhaseStatus` RPC | **cross-tenant write** |
| project | `ListCrewMembers` | `ListCrewMembers` RPC | cross-tenant read |
| project | `RemoveCrewMember` | `RemoveCrewMember` RPC | **cross-tenant delete** |
| budget | `GetLineItem` | `GetLineItem`, `UpdateLineItemActuals`, `CheckLineAvailability` RPCs | cross-tenant read |
| budget | `ListLineItems` | `ListLineItems` RPC | cross-tenant read |

`RemoveCrewMember` is a cross-tenant `DELETE` and is **not named in #157's current text** — it was found by this scan. It is the sharpest item here.

### 2.2 Latent — no caller today (2)

`inventory.GetActiveCheckout` and `budget.LockLineItem` are sqlc-generated and have **zero callers tree-wide**. Neither appears on its service's `Repository` interface. There is no live path to either.

They are in scope anyway. Both operate on tables that carry `tenant_id`, both are one line to scope, and leaving them unscoped means the first person to write a caller inherits the defect silently. Fixing them costs a `queries.sql` line and a regeneration each, with no interface or call-site churn.

### 2.3 Second-order — reachable only through a class-1 hole (1)

`budget.UpdateBudgetTotals` is `UPDATE budgets SET total_amount = (...) WHERE id = $1` — an unscoped write to the parent table. It is called only from inside `CreateLineItem` and `UpdateLineItemActuals` in the repository, with `li.BudgetID` derived from a row those functions just wrote.

It is not an independent primitive: reaching it requires already being able to write a line item, which class 1 covers. It is scoped here for defence in depth, because a recompute that can retarget an arbitrary tenant's budget total is worth closing while the file is open.

### 2.4 Correctly unscoped — NOT defects (4)

**These must not be "fixed".** Recorded here so a future audit does not re-litigate them.

- **`reporting.UpsertProjectionWatermark` / `GetProjectionWatermark`.** The `projection_watermarks` table has **no `tenant_id` column at all**: `subject TEXT PRIMARY KEY`, one row per NATS subject, tracking the last processed event. This is infrastructure state for the projection consumer, not tenant data. Adding a tenant would be wrong, not merely unnecessary.

- **`ledger.CreateJournalLine` / `ListJournalLines`.** Child rows reached only through a tenant-scoped parent. `ListJournalLines(je.ID)` runs *after* `GetJournalEntry(ctx, tenantID, id)` has already succeeded, so `je.ID` is proven to belong to the caller's tenant before the id is used. `CreateJournalLine` runs inside the transaction that just inserted the header with `TenantID: je.TenantID`; its `JournalID` is not caller-supplied.

The ledger case is the instructive one. Its SQL is *indistinguishable* from project's `GetPhase` — both are `WHERE <fk> = $1` with no tenant. One is safe and one is not, and the difference lives entirely in the call graph: ledger's parent id comes from a scoped read, project's comes straight off the request. **A grep cannot make this determination; each unscoped query needs its callers traced.** That is why this triage is part of the spec rather than a mechanical sweep.

Note that ledger's safety is held by call-site discipline, the same property already flagged as fragile for `UpdateJournalStatus`. It is correct today and out of scope here; converting it to a structural guarantee is a separate change.

## 3. Design

### 3.1 Thread `tenantID` through the repository signature

Every class-1/2/3 query gains a `tenant_id` predicate, and every repository method that runs one gains a `tenantID uuid.UUID` parameter:

```sql
-- name: GetPhase :one
SELECT * FROM phases WHERE id = $1 AND tenant_id = $2;
```

```go
func (p *Postgres) GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*project.Phase, error)
```

This matches the convention already present in the same files — `GetProduction(ctx, tenantID, id)`, `ListProductions(ctx, tenantID, ...)`, `GetJournalEntry(ctx, tenantID, id)` all have this exact shape. The change makes the child-row methods look like their parents, rather than introducing a new mechanism.

**The reason for this shape over a service-layer ownership check is the guard-by-type principle** (`feedback-enforce-guards-by-type`): a check that returns only `error` gets skipped — `RequirePermission` was omitted behind `if h.perm != nil` at 20 call sites for a year. A required `tenantID` parameter cannot be skipped, because there is nothing to pass without it. It also avoids the extra round trip that load-then-compare would add to every read.

**No migration.** `phases`, `crew_members`, `budget_line_items` and `asset_checkouts` all already declare `tenant_id UUID NOT NULL`. The columns exist and are populated; only the queries ignore them.

### 3.2 Error semantics fall out of the predicate

A cross-tenant id yields zero rows, so the existing `pgx.ErrNoRows` branch maps it to `ErrPhaseNotFound` / `ErrLineItemNotFound` → `codes.NotFound`. This is the desired answer: `NotFound` rather than `PermissionDenied`, so the response does not confirm that the row exists in some other tenant. No handler error mapping changes.

For the two `:exec` mutations (`UpdatePhaseStatus`, `RemoveCrewMember`), the existing `ErrNoRows` handling already distinguishes "no row matched" from success, so a cross-tenant attempt returns `NotFound` rather than silently affecting zero rows. This must be confirmed per query during implementation, not assumed — an `:exec` that does not check its row count would fail open.

### 3.3 Handlers supply the tenant they already hold

`GetProduction` is the model, and the guard order is already established:

```go
tenantID, ok := tenant.IDFromContext(ctx)
if !ok {
    return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
}
if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
    return nil, err
}
id, err := uuid.Parse(req.GetId())
...
prod, err := h.svc.GetProduction(ctx, tenantID, id)
```

**tenant → permission → parse → service.** Slice C installed the permission step in these handlers; this slice adds the tenant step where it is missing and threads the value down.

**Eight handlers currently have no `tenant.IDFromContext` block at all** — they go from the gate straight to `uuid.Parse`. Each needs the block added at the top, above the existing gate:

| service | handlers missing the tenant block |
|---|---|
| project | `ListPhases`, `UpdatePhaseStatus`, `ListCrewMembers`, `RemoveCrewMember` |
| budget | `GetLineItem`, `ListLineItems`, `UpdateLineItemActuals`, `CheckLineAvailability` |

`CreatePhase` and `AddCrewMember` already have the block and already thread `tenantID` into the row they write — which is why the child tables are correctly *populated* with a tenant while being incorrectly *queried* without one. The data is fine; only the read and update paths ignore it.

Adding the tenant block ahead of the gate changes the status code returned to a caller with a permission but no tenant, from `PermissionDenied` to `Unauthenticated`. This matches every other RPC in both handlers and is the intended order; it is called out because it will flip existing test expectations (§5).

### 3.4 project's handler stops reaching into the repository

`services/project/handler.go` reaches `h.svc.repo.*` directly at three sites — `:216` (`GetPhase` after add), `:235` (`ListPhases`), `:261` (`GetPhase` after status update). It is the only handler in the tree that bypasses its `Service`, and #157 names the coupling as part of the defect.

Those three lines must change regardless once the signature gains `tenantID`, so the layering is fixed at the same time. Two thin pass-throughs are added:

```go
func (s *Service) GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*Phase, error) {
	return s.repo.GetPhase(ctx, tenantID, id)
}

func (s *Service) ListPhases(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]Phase, error) {
	return s.repo.ListPhases(ctx, tenantID, productionID, limit, offset)
}
```

This is not opportunistic refactoring: it gives each scoped operation one chokepoint instead of three call sites, which is what makes the next reviewer able to verify the fix by reading `service.go`.

### 3.5 What does not change

No migration. No proto change — no field is added, removed or renumbered, so `buf breaking` is unaffected and `git diff --stat gen/` must be empty. No new permission string, so no `seedSystemRoles` edit and **no D10 backfill**. No change to the four correctly-unscoped queries in §2.4.

## 4. Interface implementers — the whole-tree hazard

Widening a `Repository` method breaks every implementer, and the implementers are not all where they appear to be. Measured:

**`project.Repository` has four implementers:**
1. `db.Postgres` — `services/project/db/postgres.go`
2. `mockRepo` — `services/project/service_test.go`
3. `projectMock` — `tests/integration/vertical/mocks_test.go`
4. `phaseReturnMock` — `tests/integration/vertical/mocks_test.go`

**`budget.Repository` has three:** `db.Postgres`, `mockRepo` (`services/budget/service_test.go`), `budgetMock` (`tests/integration/vertical/mocks_test.go`).

`tests/integration/vertical/mocks_test.go` is the hidden one — a different directory tree from the service, holding doubles for **two** of the affected services, and holding two separate project doubles. This is the same trap recorded for `iam.Repository`, where a hidden e2e double in `e2e/critical_path/helpers_test.go` was missed by focused testing and caught only by CI.

**Therefore `go test ./services/project/...` and `go build` are both insufficient signals.** The gate is whole-tree `go vet ./...`, which compiles test files across every package including `tests/integration/`. Run it before declaring any task complete.

`LockLineItem` and `GetActiveCheckout` are *not* on any `Repository` interface — they exist only in generated code. Scoping them changes `queries.sql` and the regenerated output, with no interface or call-site churn.

## 5. Testing

### 5.1 The cross-tenant denial test

Each fixed live query gets a test proving a caller in tenant A cannot reach a row in tenant B. The assertion is on the **repository argument**, not only the status code: the mock's method records the `tenantID` it received, and the test asserts it equals the caller's tenant and not the target's.

**The denial-test rule, earned in slice A and re-earned in slice C:** a denial test must trip on the first repository call the path must never reach, and the status code it asserts must not be reachable by another route. Here the specific hazard is that `NotFound` is also the natural answer for a genuinely missing row — so a test that only asserts `NotFound` proves nothing, because it passes against the unscoped code whenever the mock returns `ErrNoRows` by default. **The assertion must be on the tenant value the repository received.**

### 5.2 Existing tests that will flip

Adding a `tenant.IDFromContext` block above the gate in the eight handlers of §3.3 means any existing test calling those RPCs without a tenant in context now gets `Unauthenticated` instead of reaching the gate or the parse. Slice C's experience applies directly: **predict the count by reading the tests before changing the handler, and if the actual count differs, stop and report.** Slice C predicted 3 flips in reporting and got 5; the discrepancy was benign but was only known to be benign because it was investigated rather than absorbed.

Repair by supplying the tenant the test always should have had — never by weakening an assertion.

### 5.3 Teeth check per task

In a scratch `git worktree` **outside** the repo at the task's parent commit, copy the task's test file over and confirm every new cross-tenant test fails against the unscoped code. A test that passes against the vulnerable version is a tautology. Remove the worktree afterwards and confirm `git worktree list` is clean.

Because this task changes repository signatures, the parent commit will not compile against the new test file. The teeth check therefore runs against a hand-adapted copy: keep the test body, revert the call to the old signature. If that is impractical for a given test, say so in the task report rather than skipping the check silently.

### 5.4 Coverage

`project`, `budget` and `inventory` are in the `others ≥ 75%` tier. Record the baseline per service before starting; do not regress. Note that coverage is a poor signal here — the lines are already executed by happy-path tests and the gap is in assertions, the same way #144's and #146's guarding tests moved coverage by 0.0pp.

## 6. Constraints

- Security change touching the tenant boundary. Senior review; 2 approvals.
- **No Docker, no database.** Never run `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it once destroyed unrelated MinIO dev data. For any DB verification use a disposable, uniquely-named throwaway container or `pkg/testdb` (integration tests SKIP without `THITTAM_TEST_DSN`). CI's real-Postgres job is the authoritative gate. **This binds delegated subagents — state it in their instructions.**
- `sqlc` v1.26.0 **is** installed locally, so `make generate-sqlc` works. Regenerated `queries.sql.go` must be committed alongside the `queries.sql` change, in the same commit.
- Whole-tree `go vet ./...` is the gate, for the reason in §4.
- A signature change and every call site must land in the **same commit**, or that commit does not build and the branch is un-bisectable. (The #149 Task 3 lesson.)
- `errcheck` runs in CI; `golangci-lint` is not installed locally.
- No migration. No new permission string. No proto change. `git diff --stat gen/` must be empty — note that `gen/` here means the protobuf output, which is untouched; sqlc's output lives in `services/*/db/queries.sql.go` and **will** change.
- `gh pr checks` before declaring the PR ready.

## 7. Out of scope

- **The remaining #139 gating slices** — B (iam completion), D (expense reads + the D10 backfill), E/F/G (document, billing, notifications). This slice adds no permission gate and revives no vocabulary.
- **Converting ledger's call-site-discipline safety into a structural guarantee** (§2.4). Correct today; a separate change.
- **`UpdateJournalStatus`'s unscoped compensating SELECT**, whose error is discarded. Known, not exploitable, deserves its own issue.
- **Row-level security.** Considered and rejected for this slice: it is the strongest option, but needs a migration, per-checkout session-variable wiring in the pool, and RLS-aware tests, and it is untestable locally under the no-database constraint. Worth revisiting as a platform-wide control once the per-query fixes have established which tables are tenant-scoped.
- **Whether `X-Tenant-ID` is itself trustworthy.** These handlers scope from `tenant.IDFromContext` — the header path — not from the verified token claim. This slice makes the queries honour whatever tenant the context carries; it does not certify how that tenant got there. That is #139 §3 / slice H.
