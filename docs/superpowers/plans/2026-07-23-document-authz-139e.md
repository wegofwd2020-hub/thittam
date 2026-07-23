# document Service Authorization Implementation Plan (#139 slice E)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire fail-closed authorization into the `document` service and gate its thirteen ungated RPCs on three new permissions (`document:read` / `document:write` / `document:delete`), with a migration and seed updates granting them to existing and new tenants.

**Architecture:** Three tasks. Task 1 creates the vocabulary — a migration, a `systemRoles` edit, and two seed-fixture updates — and **must land first** (gating a permission nobody holds locks everyone out). Task 2 wires the fail-closed `perm` field and IAM dial into `document`, mirroring `reporting` (slice C), and gates all thirteen RPCs. Task 3 is the migration's integration test plus the `systemRoles` grant test.

**Tech Stack:** Go 1.25, golang-migrate, pgx/v5, grpc-go, testify.

**Spec:** `docs/superpowers/specs/2026-07-23-document-authz-139e-design.md` (committed on this branch at `ec395db`). Read §3 (grant matrix), §4 (the billing coupling — why reads must stay wide), and §5 (the wiring).

## Global Constraints

- **Branch:** `fix/document-authz-139e`, already created off a merged `main` that includes slice D (migration 020). This branch adds migration **021**.
- **NO Docker, NO database.** NEVER run `docker compose` with `-v`, `down`, or `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it destroyed unrelated MinIO dev data once. Use `pkg/testdb` (skips without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. **CI's `Migration Validate (up + down)` and `Integration Tests (real Postgres)` jobs are the authoritative gates.**
- **`Migration Validate` runs against a freshly created empty database**, so it validates 021's syntax and that the down file doesn't error — NOT the grant matrix or idempotency. Those are proven solely by the integration test (Task 3).
- **This slice has a migration** — `migrations/` WILL show a diff. `gen/` must stay empty.
- **Do NOT run `make generate-sqlc`.** Nothing here touches `queries.sql`; codegen is repo-wide and would dirty `services/billing/` with pre-existing drift (issue #160).
- **NEVER `git add -A`.** Use the scoped `git add` in each task's commit step.
- **Whole-tree `go vet ./...` is the completion gate for every task.**
- **Guard order: tenant (`TenantFromRequest`) → permission (`RequirePermission`) → `uuid.Parse` → service call.** The gate goes after the existing tenant block and before any parse. `interceptor` is already imported in `handler.go`.
- Permission strings are **inline literals** (`"document:read"` etc.), matching how every prior slice gated. No constants.
- **iam must NOT use `interceptor.RequirePermission`; `document` MUST.** `document` is a separate service that dials iam over gRPC — the reporting/ledger pattern, not the in-process `requireUserManage` pattern that only `iam` itself uses.
- Coverage floors per CLAUDE.md:77 — iam/general-ledger ≥ 85%, budget/expense ≥ 80%, others ≥ 75%. `document` is `others` (≥ 75%). `iam` is 87.3%.
- **Verification commands containing `$`, `(`, `)` or quotes MUST use `grep -F`.** Count flips/failures with `go test ./... 2>&1 | grep -- '--- FAIL'`, NEVER `grep -E '^\s+--- FAIL'` (matches nothing; silently reports zero).

### The three-halves rule (learned on #168)

Adding a permission string touches **three** places, not two. Miss any one and a population of tenants lacks the permission:

1. `migrations/iam/021_*.up.sql` — existing tenants.
2. `services/iam/service.go` `systemRoles` — new tenants (via `seedSystemRoles`).
3. **`seeds/demo/xyz-cba/007_iam_roles.sql` and `seeds/template/new-tenant/001_tenant.sql`** — both hardcode the seven roles and claim to mirror `systemRoles`. `make db-reset` runs `migrate-all` then `seed`, so a stale fixture silently reverts the migration for the demo tenant. #168's whole-branch review caught exactly this omission.

### The grant matrix (spec §3.1) — every task must match this exactly

| role | `document:read` | `document:write` | `document:delete` |
|---|:-:|:-:|:-:|
| super_admin | ✓ | ✓ | ✓ |
| manager | ✓ | ✓ | ✓ |
| coordinator | ✓ | ✓ | — |
| accountant | ✓ | ✓ | — |
| project_supervisor | ✓ | ✓ | — |
| member | ✓ | — | — |
| inventory_manager | ✓ | — | — |

`document:read` → **all seven** (load-bearing: the billing→document token-forward path in spec §4 requires it; narrowing breaks invoice download). `document:write` → the five operator roles. `document:delete` → super_admin, manager only.

---

### Task 1: Vocabulary — migration, systemRoles, seeds

Creates the three permission strings. **Nothing is gated here.**

**Files:**
- Create: `migrations/iam/021_seed_document_permissions.up.sql`
- Create: `migrations/iam/021_seed_document_permissions.down.sql`
- Modify: `services/iam/service.go` (the `systemRoles` var)
- Modify: `seeds/demo/xyz-cba/007_iam_roles.sql`
- Modify: `seeds/template/new-tenant/001_tenant.sql`

**Interfaces:**
- Consumes: nothing.
- Produces: the strings `"document:read"`, `"document:write"`, `"document:delete"` present on the seeded system roles per the grant matrix. Task 2 gates on those exact literals.

- [ ] **Step 1: Record the iam coverage baseline**

```bash
go test ./services/iam/ -cover -count=1 2>&1 | tail -1
```

Record the figure (expected `87.3%`); compare at Step 6.

- [ ] **Step 2: Write the up migration**

Create `migrations/iam/021_seed_document_permissions.up.sql`:

```sql
-- 021_seed_document_permissions.up.sql
-- #139 slice E: grant the three document permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change so NEW
-- tenants receive these at seedSystemRoles time; the two seed fixtures are
-- updated too. All three halves are required (see #168's review).
--
-- Idempotent by necessity: migrations/iam runs against the public schema via
-- `make migrate-all` AND against every new tenant_<uuid> at CreateTenant.
--
-- is_system = true only: custom roles are a tenant's own business.
-- document:read is granted to ALL seven roles: billing.DownloadInvoice
-- forwards the caller's token to document.GetDownloadURL, so narrowing reads
-- would break invoice download (#139 slice E spec §4).

UPDATE roles SET permissions = array_append(permissions, 'document:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'member', 'inventory_manager', 'project_supervisor')
  AND NOT ('document:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'document:write')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('document:write' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'document:delete')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('document:delete' = ANY (permissions));
```

- [ ] **Step 3: Write the down migration**

Create `migrations/iam/021_seed_document_permissions.down.sql`:

```sql
-- 021_seed_document_permissions.down.sql
-- Reverse of 021. All three strings are new to every role this migration
-- touches (unlike slice D's inventory:read, which pre-existed on
-- inventory_manager), so removing each unconditionally across is_system roles
-- is correct — there is no pre-existing grant to preserve.

UPDATE roles SET permissions = array_remove(permissions, 'document:read')   WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'document:write')  WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'document:delete') WHERE is_system = true;
```

- [ ] **Step 4: Edit `systemRoles`**

In `services/iam/service.go`, add the strings to each role's permission list per the grant matrix. Read the current block first (it is the same var slice D edited). The exact additions:

- `super_admin`: add `"document:read", "document:write", "document:delete"`
- `manager`: add `"document:read", "document:write", "document:delete"`
- `coordinator`: add `"document:read", "document:write"`
- `accountant`: add `"document:read", "document:write"`
- `project_supervisor`: add `"document:read", "document:write"`
- `member`: add `"document:read"` only
- `inventory_manager`: add `"document:read"` only

Place each next to that role's existing strings; do not reorder or reformat the rest of the block. **`member` and `inventory_manager` get `document:read` ONLY — not write, not delete.** Getting that wrong (e.g. adding write to member) is the exact class of error Task 3's grant test exists to catch.

- [ ] **Step 5: Update the two seed fixtures**

Both hardcode the seven roles. Read each file first and match its array-literal style exactly (they differ — one uses `ARRAY['a','b']` on one line, the other wraps). Add the same strings per the grant matrix, to the same seven roles, in both files. **`member`'s array gets `document:read` only; `super_admin`/`manager` get all three; the middle three get read+write.**

Do not reorder or reformat any other role. After editing, sanity-check with:

```bash
grep -cF 'document:read' seeds/demo/xyz-cba/007_iam_roles.sql      # expect 7
grep -cF 'document:write' seeds/demo/xyz-cba/007_iam_roles.sql     # expect 5
grep -cF 'document:delete' seeds/demo/xyz-cba/007_iam_roles.sql    # expect 2
grep -cF 'document:read' seeds/template/new-tenant/001_tenant.sql  # expect 7
grep -cF 'document:write' seeds/template/new-tenant/001_tenant.sql # expect 5
grep -cF 'document:delete' seeds/template/new-tenant/001_tenant.sql# expect 2
```

If any count is off, a role was missed or double-added — fix before committing.

- [ ] **Step 6: Verify**

```bash
go vet ./...
go test ./services/iam/ -race -cover -count=1
git diff --stat gen/          # must be empty
git diff --stat migrations/   # EXPECTED non-empty: the two 021 files
```

**Flip prediction: exactly 2**, measured against the current tests:

```bash
go test ./services/iam/ -count=1 2>&1 | grep -- '--- FAIL' | sort
```

`services/iam/service_test.go` has four `TestSystemRoles_*` tests using `assert.ElementsMatch`, but only two pin a role's **whole** permission list, and those are the two that flip:

- `TestSystemRoles_InventoryManagerPermissions` (:837) — `inventory_manager` gains `document:read`.
- `TestSystemRoles_ProjectSupervisorPermissions` (:850) — `project_supervisor` gains `document:read` + `document:write`.

The other two — `TestSystemRoles_LedgerGrants` (:2327) and `TestSystemRoles_ReadGrants` (:2372) — are **namespace-filtered**: each collects only `ledger:` / `expense:read`+`inventory:read` strings before asserting, so `document:*` additions are invisible to them and they do NOT flip. (Verified by reading both.)

Repair the two whole-list tests by adding the new `document:*` strings to their expected `ElementsMatch` literal per the grant matrix — `document:read` to inventory_manager, `document:read`+`document:write` to project_supervisor. Keep `ElementsMatch`; never weaken to a subset check. **If any test flips that is NOT one of those two, STOP and report** — it would mean the edit changed behaviour beyond role contents. (Slice D's Task 1 predicted zero and got one; reconcile against the actual run regardless.)

- [ ] **Step 7: Commit**

```bash
git add migrations/iam/021_seed_document_permissions.up.sql \
        migrations/iam/021_seed_document_permissions.down.sql \
        services/iam/service.go \
        services/iam/service_test.go \
        seeds/demo/xyz-cba/007_iam_roles.sql \
        seeds/template/new-tenant/001_tenant.sql
git commit -m "feat(iam): grant document:read/write/delete to system roles (#139)

Creates the vocabulary slice E's gates need. Nothing is gated in this
commit -- gating a permission nobody holds locks everyone out, so the
grants land first.

Three strings, splitting destructive from ordinary writes: document:read
(all 7 roles), document:write (5 operator roles), document:delete
(super_admin, manager). read is granted to all seven because
billing.DownloadInvoice forwards the caller's token to
document.GetDownloadURL, so narrowing it would break invoice download.

All three halves updated: migration for existing tenants, systemRoles for
new ones, and both seed fixtures (seeds/demo/xyz-cba and
seeds/template/new-tenant) which hardcode the roles and would otherwise
revert the migration under make db-reset.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Wire fail-closed authorization and gate all thirteen RPCs

Mirrors `reporting` (slice C): `document` gains a required `perm` field, `cmd/document` dials IAM and refuses to start without it, and every RPC gets a gate. **The signature change and the `cmd/` wiring land in the same commit** — otherwise that commit does not build (#149 Task 3 lesson).

**Files:**
- Modify: `services/document/handler.go` (struct, `NewHandler`, all 13 RPCs)
- Modify: `cmd/document/main.go` (IAM dial + `NewHandler` call)
- Modify: `services/document/handler_test.go` (8 construction sites + new denial tests)

**Interfaces:**
- Consumes: the three `"document:*"` strings from Task 1.
- Produces: `NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler` — the new required signature.

- [ ] **Step 1: Record the document coverage baseline**

```bash
go test ./services/document/ -cover -count=1 2>&1 | tail -1
```

- [ ] **Step 2: Add the `perm` field and require it in `NewHandler`**

In `services/document/handler.go`:

```go
type Handler struct {
	documentv1.UnimplementedDocumentServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

// NewHandler creates a document handler. perm is required, not optional:
// forgetting it is a build error, and cmd/document refuses to start when the
// checker is nil. document dials iam over gRPC (the reporting/ledger pattern),
// so it uses interceptor.RequirePermission — not iam's in-process helper.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}
```

`go build ./...` now fails at every construction site — that is the point. The next steps fix all eight.

- [ ] **Step 3: Wire `cmd/document/main.go` (same file-edit, will be same commit)**

In `cmd/document/main.go`, replace `handler := document.NewHandler(svc)` (around line 80) and add the IAM dial above it, mirroring `cmd/reporting-analytics/main.go:92-101`:

```go
	iamPerm, closeIAM, err := iamclient.DialFromEnv("document")
	if err != nil {
		log.Fatalf("document: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()
	if iamPerm == nil {
		log.Fatalf("document: startup: %s is not set; document cannot authorize without a permission checker", iamclient.EnvAddr)
	}
	handler := document.NewHandler(svc, iamPerm)
```

Add the import `"github.com/wegofwd2020/thittam/pkg/iamclient"` (read `cmd/reporting-analytics/main.go`'s import block for the exact path). `iamPerm` is the concrete `*iamclient.PermissionChecker` at the nil check, so this is a plain pointer comparison, not the typed-nil trap.

- [ ] **Step 4: Fix the two test helpers, then write the denial-test doubles**

`services/document/handler_test.go` has **8 `NewHandler` construction sites**: two helpers (`newHandler()` ~:16, `newHandlerWithRepo(r)` ~:23) and six inline (~:145, :180, :196, :298, :427, :559). First add the two permission doubles at the top of the file (the package has neither today), mirroring `services/project/handler_test.go`:

```go
// allowAllPerm grants every permission; authz semantics are covered by
// pkg/interceptor's own tests.
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm denies every permission, so a denial test proves the gate fires
// before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}
```

Then thread `allowAllPerm{}` through the two helpers:

```go
func newHandler() *Handler {
	return NewHandler(newTestService(&mockRepo{}), allowAllPerm{})
}

func newHandlerWithRepo(r *mockRepo) *Handler {
	return NewHandler(newTestService(r), allowAllPerm{})
}
```

And append `, allowAllPerm{}` to each of the six inline `NewHandler(newTestService(&mockRepo{...}))` calls. After this, `go build ./...` and the existing tests compile again with no behaviour change.

- [ ] **Step 5: Write the thirteen denial tests**

One per RPC. Each installs `t.Fatal` on **the first repository fn its happy path reaches** — traced from the service layer, since the mock field names do not match RPC names. The exact tripwire per RPC:

| RPC | permission | tripwire mock fn |
|---|---|---|
| `GetDocument` | `document:read` | `getDocumentFn` |
| `ListDocuments` | `document:read` | `listDocumentsFn` |
| `GetDownloadURL` | `document:read` | `getDocumentFn` (calls `GetDocument` first) |
| `ListVersions` | `document:read` | `listVersionsFn` |
| `ListFolders` | `document:read` | `listFoldersFn` |
| `InitiateUpload` | `document:write` | `createDocumentFn` |
| `ConfirmUpload` | `document:write` | `getDocumentFn` |
| `MoveDocument` | `document:write` | `getFolderFn` |
| `CreateVersion` | `document:write` | `getDocumentFn` (calls `GetDocument` first) |
| `ConfirmVersion` | `document:write` | `createVersionFn` |
| `CreateFolder` | `document:write` | `createFolderFn` |
| `DeleteDocument` | `document:delete` | `getDocumentFn` |
| `RestoreVersion` | `document:delete` | `getDocumentFn` (calls `GetDocument` first) |

Template (for `GetDocument`; replicate for the other twelve with the right tripwire and request type):

```go
func TestHandler_GetDocument_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			t.Fatal("repository reached: GetDocument must deny before querying")
			return nil, nil
		},
	})

	_, err := h.GetDocument(ctxWithTenant(uuid.New()), &documentv1.GetDocumentRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Add a `newHandlerWithRepoDeny` helper alongside the others so the deny double is threaded once:

```go
func newHandlerWithRepoDeny(r *mockRepo) *Handler {
	return NewHandler(newTestService(r), denyPerm{})
}
```

**Confirm the exact `ctxWithTenant` helper name and each request-type/field name against the existing file before writing** — read one existing test per RPC group. If a mock fn signature differs from the table (e.g. an extra arg), match the mock, and if a tripwire fn turns out wrong when you read the service method, correct it and **report the discrepancy**.

- [ ] **Step 6: Run the denial tests to verify they fail against the ungated handlers**

Before inserting gates, the denial tests must FAIL (the handler reaches the repo, so `t.Fatal` fires). This is the teeth check.

```bash
go test ./services/document/ -run Denied -v
```

Expected: all thirteen FAIL. Record it. (If any PASSES here, its gate is missing or its tripwire is on a fn the happy path doesn't reach — fix before proceeding.)

- [ ] **Step 7: Insert the thirteen gates**

In `services/document/handler.go`, add to each RPC immediately after the `TenantFromRequest` block and before the `uuid.Parse`, with the permission from the table in Step 5:

```go
	if err := interceptor.RequirePermission(ctx, h.perm, "document:read"); err != nil {
		return nil, err
	}
```

`GetDocument` becomes:

```go
func (h *Handler) GetDocument(ctx context.Context, req *documentv1.GetDocumentRequest) (*documentv1.Document, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "document:read"); err != nil {
		return nil, err
	}
	docID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	doc, err := h.svc.GetDocument(ctx, tenantID, docID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return docToProto(doc), nil
}
```

Apply the correct permission to each of the thirteen per the Step 5 table. Some RPCs (e.g. `InitiateUpload`, `MoveDocument`) resolve the tenant with `TenantFromRequest` and may have additional parses — the gate always goes immediately after the tenant block and before the *first* parse.

- [ ] **Step 8: Run the denial tests to verify they pass, and check for flips**

```bash
go test ./services/document/ -run Denied -v          # all 13 now PASS
go test ./services/document/ -count=1 2>&1 | grep -- '--- FAIL' | sort
```

**Flip prediction: zero.** Every existing test routes through `newHandler()`/`newHandlerWithRepo()` (now carrying `allowAllPerm{}`) or an inline site you fixed in Step 4, so all pass the gate. **If any existing (non-denial) test fails, STOP and report** — it means a test built a handler a way Step 4 missed, or a gate landed on the wrong permission.

- [ ] **Step 9: Full check**

```bash
go vet ./...
go build ./cmd/...
go test ./services/document/ -race -cover -count=1
grep -cF 'interceptor.RequirePermission(ctx' services/document/handler.go   # must be 13
git diff --stat gen/ migrations/   # both empty this task
```

Coverage ≥ the Step 1 baseline; `document` floor is 75%.

- [ ] **Step 10: Commit**

```bash
git add services/document/handler.go cmd/document/main.go services/document/handler_test.go
git commit -m "fix(document): wire fail-closed authz and gate all 13 RPCs (#139)

document had 13 RPCs, every one ungated, and no permission checker at all.
This adds the reporting/ledger pattern: NewHandler takes a required perm
param, and cmd/document refuses to start without IAM_SERVICE_ADDR -- the
third fail-closed service after ledger and reporting.

Thirteen gates: document:read on the 5 reads, document:write on the 6
writes, document:delete on DeleteDocument and RestoreVersion (which
overwrites the current version, a destructive act). Guard order is
tenant -> permission -> parse.

The NewHandler signature change and its cmd/document call site land in one
commit so the branch stays bisectable (the #149 Task 3 lesson).

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Migration and grant tests

Proves the migration is idempotent and pins the grant matrix on the `systemRoles` side, mirroring what #168 established for slice D.

**Files:**
- Create: `services/iam/db/document_permission_backfill_integration_test.go`
- Modify: `services/iam/service_test.go` (add `TestSystemRoles_DocumentGrants`)

**Interfaces:**
- Consumes: migration 021 and the `systemRoles` edit from Task 1.
- Produces: nothing.

- [ ] **Step 1: Write the `systemRoles` grant test**

In `services/iam/service_test.go`, add `TestSystemRoles_DocumentGrants`. **Read `TestSystemRoles_LedgerGrants` first** (slice D used it as the template) — it iterates `systemRoles` with a want-map and an explicit none-list. Mirror its shape:

```go
func TestSystemRoles_DocumentGrants(t *testing.T) {
	t.Parallel()
	// Expected document:* grants per role (spec §3.1). Absence is as
	// load-bearing as presence: member must NOT gain write or delete.
	want := map[string][]string{
		"super_admin":        {"document:read", "document:write", "document:delete"},
		"manager":            {"document:read", "document:write", "document:delete"},
		"coordinator":        {"document:read", "document:write"},
		"accountant":         {"document:read", "document:write"},
		"project_supervisor": {"document:read", "document:write"},
		"member":             {"document:read"},
		"inventory_manager":  {"document:read"},
	}

	byName := map[string][]string{}
	for _, r := range systemRoles {
		byName[r.name] = r.permissions
	}

	for role, expected := range want {
		perms := byName[role]
		for _, p := range expected {
			assert.Contains(t, perms, p, "%s must hold %s", role, p)
		}
		// Nothing outside `expected` from the document: namespace.
		for _, p := range perms {
			if len(p) >= 9 && p[:9] == "document:" {
				assert.Contains(t, expected, p, "%s holds unexpected %s", role, p)
			}
		}
	}
}
```

This fails if anyone adds `document:write` to `member`, or drops a grant from a role that should have it.

- [ ] **Step 2: Run the grant test**

```bash
go test ./services/iam/ -run TestSystemRoles_DocumentGrants -v
```

Expected: PASS (Task 1 already edited `systemRoles`). If it FAILS, Task 1's edit and the matrix disagree — reconcile against the spec §3.1 matrix, and if `service.go` is wrong fix it there, not by loosening the test.

- [ ] **Step 3: Write the migration integration test**

Create `services/iam/db/document_permission_backfill_integration_test.go`. **First line must be the build tag, then a blank line** — read `services/iam/db/role_permission_backfill_integration_test.go` (slice D's, on this branch's `main`) and copy its `testdb` setup and structure exactly.

Cover three properties:
1. `document:write` is appended to a granted role (e.g. `manager`) and applying the statement twice appends it only once (idempotency).
2. `document:delete` is NOT appended to a role outside its grant set (e.g. `coordinator`) — proving the `name IN (...)` list is enforced.
3. `member` receives `document:read` but NOT `document:write` or `document:delete`.

Copy the exact `UPDATE` statements from `migrations/iam/021_seed_document_permissions.up.sql` into consts, with a comment to keep them in sync, exactly as slice D's test does.

- [ ] **Step 4: Verify the tagged file compiles and check local skip**

```bash
go vet -tags=integration ./services/iam/db/
go test -tags=integration ./services/iam/db/ -run Document -v
```

The first MUST be clean — it is the only local signal the tagged file builds. The second SKIPs without `THITTAM_TEST_DSN`; **that is the expected local outcome and must be reported as a SKIP, not a pass.** Do not stand up a database. Proof is deferred to CI's real-Postgres job.

- [ ] **Step 5: Full check**

```bash
go vet ./...
go test ./services/iam/ -race -cover -count=1
git diff --stat gen/   # empty
```

Coverage on `services/iam` ≥ 87.3% baseline (floor 85%).

- [ ] **Step 6: Commit**

```bash
git add services/iam/db/document_permission_backfill_integration_test.go services/iam/service_test.go
git commit -m "test(iam): cover migration 021 idempotency and the document grant matrix (#139)

CI's Migration Validate runs against an empty database, so it proves only
021's syntax. The integration test here is the sole check that the grants
land idempotently, that the name IN (...) lists are enforced, and that
member gets document:read but not write or delete.

TestSystemRoles_DocumentGrants pins the matrix on the code side: adding
document:write to member on the systemRoles side now fails a test rather
than shipping silently -- the gap #168's review found for slice D.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Whole-branch verification

Run after all three tasks, before opening the PR.

- [ ] **Step 1: Build and vet**

```bash
go vet ./...
go vet -tags=integration ./services/iam/db/
go build ./cmd/...
```

- [ ] **Step 2: Suites**

```bash
go test ./... -short -count=1
go test ./services/document/ ./services/iam/ -race -count=1
```

- [ ] **Step 3: End state**

```bash
grep -cF 'interceptor.RequirePermission(ctx' services/document/handler.go   # 13
grep -cF 'document:read' services/iam/service.go                            # 7
grep -cF 'document:write' services/iam/service.go                           # 5
grep -cF 'document:delete' services/iam/service.go                          # 2
ls migrations/iam/021_seed_document_permissions.*.sql                       # both files
grep -cF 'iamclient.DialFromEnv' cmd/document/main.go                       # 1
```

Every pattern here contains `(` or `:` — all use `grep -F`.

- [ ] **Step 4: Constraints**

```bash
git diff --stat main..HEAD -- gen/   # empty
git status --short                   # clean; services/billing/ must NOT appear
git diff --stat main..HEAD -- migrations/   # the two 021 files
```

- [ ] **Step 5: Coverage**

```bash
go test ./services/document/ ./services/iam/ -cover -count=1
```

`document` ≥ 75%, `iam` ≥ 85%.

- [ ] **Step 6: Push and open the PR**

```bash
git push -u origin fix/document-authz-139e
```

PR body must state:

- 13 RPCs gated; `document` becomes the third fail-closed service (ledger, reporting, document).
- Three new permission strings; grant matrix per spec §3.1.
- **`document:read` granted to all seven roles is deliberate and load-bearing** — `billing.DownloadInvoice` forwards the caller's token to `document.GetDownloadURL`, so narrowing reads would break invoice download for whoever lacks the permission, surfacing as an opaque error inside billing. Slice E changes no existing read behaviour.
- **DEPLOY ORDERING: migration 021 must run BEFORE the new document code.** Worse than slice D here: `document` is *also* becoming fail-closed, so if the code deploys first, existing tenants lose all thirteen document RPCs (not degraded — hard `PermissionDenied`) until the migration runs. `IAM_SERVICE_ADDR` must be present in the document pod's environment before rollout (it is in `thittam-common`). #166 tracks that nothing enforces the ordering.
- `Migration Validate` is a syntax gate only (empty DB); the grant matrix and idempotency are proven by the Task 3 integration test in CI's real-Postgres job.
- Flag for senior review — security change, 2 approvals.

- [ ] **Step 7: Confirm CI**

```bash
gh pr checks <number>
```

**Local green is not CI green.** `Migration Validate (up + down)` and `Integration Tests (real Postgres)` cannot run locally and are the only checks that exercise the migration and its idempotency test. Do not declare ready until both pass.
