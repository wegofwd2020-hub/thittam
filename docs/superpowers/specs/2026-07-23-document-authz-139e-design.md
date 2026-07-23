# document service authorization — full wiring, new vocabulary, and a cross-service coupling — design

**Issue:** #139, slice E. **Branch:** `fix/document-authz-139e`, base = a merged `main` that includes slice D (#168). **Not yet created** — see §2.
**Follows:** #138, #144, #146, #149, slice A (#155), slice C (#158), #157 (#161), slice B (#164), slice D (#168).
**Policy table:** `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`.

## 1. What this slice is

`document` has **thirteen RPCs, every one ungated**. It is the first of the three zero-authorization services (E document, F billing, G notifications) and the largest single-service slice in #139. It combines both hard problems earlier slices solved separately:

- Like `reporting` (slice C), it has **no `perm` field, no `WithPermissionChecker`, and no IAM dial** — it needs the full authorization wiring, not just gate insertions.
- Like `expense` (slice D), **no `document:*` vocabulary exists** — it needs new permission strings, a migration, and the seed-fixture updates.

It also has something no earlier slice had: a **cross-service caller**. `billing.DownloadInvoice` forwards the end user's token to `document.GetDownloadURL` (§4), so the grant set for reads is constrained by who must be able to download an invoice.

Every RPC is already tenant-bounded (`tenant.IDFromContext` or `TenantFromRequest`) and `document` already authenticates via `UnaryAuthInterceptor`, so the caller is present in context. This slice adds authorization on top of authentication that already works.

## 2. Sequencing — this branch waits for slice D

Slice E needs migration **021**. `main` currently ends at **019**; slice D's PR #168 adds **020**. Branching E off today's `main` would either collide on the number or duplicate slice D's work in the diff.

**This branch is created only after #168 merges to `main`.** The spec and plan are written now (they need no branch); the branch, and migration 021, come after. This keeps slice E's diff to slice E's changes and avoids a stacked-branch rebase.

## 3. Vocabulary and grants — three strings

`document` gets three permission strings, splitting destructive acts from ordinary writes the way `ledger` splits read/write/post/admin. Deleting a signed contract or rolling a version back is not the same risk as uploading a call sheet.

| permission | RPCs |
|---|---|
| `document:read` | `GetDocument`, `ListDocuments`, `GetDownloadURL`, `ListVersions`, `ListFolders` |
| `document:write` | `InitiateUpload`, `ConfirmUpload`, `MoveDocument`, `CreateVersion`, `ConfirmVersion`, `CreateFolder` |
| `document:delete` | `DeleteDocument`, `RestoreVersion` |

`RestoreVersion` is classed `delete`, not `write`: it overwrites the current version with an older one, destroying the intervening state — a destructive act, not an additive one.

### 3.1 Grant matrix

| role | `document:read` | `document:write` | `document:delete` |
|---|:-:|:-:|:-:|
| super_admin | ✓ | ✓ | ✓ |
| manager | ✓ | ✓ | ✓ |
| coordinator | ✓ | ✓ | — |
| accountant | ✓ | ✓ | — |
| project_supervisor | ✓ | ✓ | — |
| member | ✓ | — | — |
| inventory_manager | ✓ | — | — |

**`document:read` → all seven roles.** Documents are production working artifacts — call sheets, contracts, receipts, invoices — that every role legitimately touches, and the tenant boundary is the real control. This is decision D3 applied to a resource everyone uses. It is also **required for correctness**, not just convenience: `billing.DownloadInvoice` forwards the caller's token to `GetDownloadURL` (§4), and that path works for any authenticated member today. Granting reads to all seven means slice E **changes no existing read behaviour** — it only adds a check that the current population already passes.

**`document:write` → the five operator roles**, matching `expense:read` / `budget:read`'s shape (super_admin, manager, coordinator, accountant, project_supervisor). `member` and `inventory_manager` are excluded: nothing in the tree has a member or inventory-manager upload documents through this service (billing is the only cross-service caller, and it only reads). If a member-upload flow is added later, that is a grant change, not a code change.

**`document:delete` → super_admin and manager only.** Deleting stored files and rolling back versions is the platform's most destructive document operation; it is held by the two roles that already hold every destructive permission in their services. `coordinator` writes but does not delete — the same split `ledger` draws between `write` and `admin`.

**This is a proposal open to revision at spec review.** The read grant is load-bearing (§4) and should not narrow. The write/delete split is a judgment call; if the product intent is that coordinators delete, that is a one-line matrix change.

## 4. The billing → document coupling — why reads must stay wide

`cmd/billing/main.go:100` dials `document` with `ForwardAuthUnaryClientInterceptor` (#138), so the **end user's token** — not a service token — reaches `document`. `billing.DownloadInvoice` (`services/billing/handler.go:170`) calls `docClient.GetDownloadURL`.

Two consequences:

1. **`DownloadInvoice` is itself ungated** (billing is 0/14; slice F). It is tenant-bounded but any authenticated member can call it today. So `GetDownloadURL` is reachable, with a real user token, by any authenticated member.
2. **Gating `GetDownloadURL` narrowly would break invoice download** for whoever lacks the permission — and the failure would surface *inside billing* as an opaque downstream error, not at the RPC the user called.

Granting `document:read` to all seven roles resolves this: the population that can download an invoice today is exactly the population that holds `document:read` after this slice. **No manifest or billing change is needed** — the forwarding interceptor already sends the token document now requires.

When slice F gates `DownloadInvoice`, the natural gate is a billing permission; document's own check remains the backstop. The two are independent and both correct.

## 5. Wiring — mirror reporting (slice C)

`document` needs the fail-closed startup that #149 built for ledger and slice C replicated for reporting.

```go
type Handler struct {
	documentv1.UnimplementedDocumentServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

// NewHandler creates a Handler. perm is required, not optional: forgetting it
// is a build error, and cmd/document refuses to start when the checker is nil.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}
```

`cmd/document/main.go` gains the IAM dial and refuse-to-start, exactly as `cmd/reporting-analytics/main.go:92-101`:

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

`iamPerm` is the concrete `*iamclient.PermissionChecker` at the nil check, so this is a plain pointer comparison, not the typed-nil trap. `document` already authenticates (`cmd/document/main.go:92`), so the caller is present in context and `RequirePermission` has a `caller.UserID` to check.

This makes `document` the **third fail-closed service** (ledger, reporting, document); project, budget, expense, inventory remain log-and-proceed (#167 tracks converting them).

**`IAM_SERVICE_ADDR` must reach the document pod.** It is already in the shared `thittam-common` ConfigMap that every service consumes; confirm document's deployment references it during implementation, but no new manifest key is expected.

## 6. The migration and seeds — the slice D pattern exactly

`migrations/iam/021_seed_document_permissions.{up,down}.sql`, idempotent, `is_system = true` only, one `UPDATE` per permission per role-set:

```sql
-- up
UPDATE roles SET permissions = array_append(permissions, 'document:read')
WHERE is_system = true
  AND name IN ('super_admin','manager','coordinator','accountant','member','inventory_manager','project_supervisor')
  AND NOT ('document:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'document:write')
WHERE is_system = true
  AND name IN ('super_admin','manager','coordinator','accountant','project_supervisor')
  AND NOT ('document:write' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'document:delete')
WHERE is_system = true
  AND name IN ('super_admin','manager')
  AND NOT ('document:delete' = ANY (permissions));
```

The down migration removes all three from every `is_system` role unconditionally — all three strings are new to every role this slice touches, so unlike slice D's `inventory:read` there is **no pre-existing grant to preserve** and no `inventory_manager`-style exclusion.

**Three halves, not two** (the lesson slice D's review caught): the migration covers existing tenants, the `systemRoles` edit in `services/iam/service.go` covers new tenants, and **the two seed fixtures must be updated too** —

- `seeds/demo/xyz-cba/007_iam_roles.sql`
- `seeds/template/new-tenant/001_tenant.sql`

both hardcode the seven roles and claim to mirror `systemRoles`. Add the three strings per the §3.1 matrix. Omitting them leaves `make db-reset` producing a demo tenant that fails every gated document RPC — exactly the defect #168 fixed for expense/inventory.

## 7. Design — the gates

Thirteen gate insertions, guard order **tenant → permission → parse**, immediately after the existing tenant block and before any `uuid.Parse`, matching every prior slice. Permission strings are inline literals.

No repository or service-layer signature changes. No proto change. `document`'s handlers already resolve the tenant; this adds one gate line each.

## 8. Testing

**Denial tests** — one per gated RPC, `t.Fatal` on the first repository fn that RPC calls on its happy path, confirmed by reading the handler. `document` has neither `allowAllPerm` nor `denyPerm` today (it had no checker); both doubles are added, mirroring `services/project/handler_test.go`.

**This is a compile break, not a set of runtime flips** — a distinct discipline from slices B and D. Once `NewHandler` requires `perm`, every construction site stops compiling until it passes a checker. Measured: `services/document/handler_test.go` has **8 `NewHandler(newTestService(...))` sites** — two helpers (`newHandler()` :16, `newHandlerWithRepo(r)` :23) plus six tests that build a mock inline (:145, :180, :196, :298, :427, :559). Fixing the two helpers to thread `allowAllPerm{}` covers every test that routes through them; the six inline sites each need the argument added directly. The plan enumerates all eight. Because it is a compile error, there is no "flip count" to predict at runtime — `go build ./...` either succeeds or names the exact unfixed site. A test that was asserting real behaviour keeps asserting it; only construction changes.

Note the production call site — `cmd/document/main.go` — is the eighth-and-most-important construction site and must change in the **same commit** as the signature (§9, the #149 lesson), or that commit does not build.

**The migration's integration test** — idempotency for all three strings, applied twice, plus a `member`-gets-only-`document:read` assertion, mirroring slice D's `role_permission_backfill_integration_test.go`. Carries `//go:build integration`; `go vet -tags=integration ./services/iam/db/` is the only local signal it compiles; it SKIPs without `THITTAM_TEST_DSN`; CI's real-Postgres job is the authoritative gate.

**A `systemRoles` grant test** — `TestSystemRoles_DocumentGrants`, mirroring `TestSystemRoles_LedgerGrants` / the `ReadGrants` test slice D added: a want-map plus an explicit none-list, so adding `document:write` to `member` on the code side fails a test rather than silently shipping.

**The billing coupling needs a guard against regression.** `services/billing`'s existing `DownloadInvoice` test uses a `docClient` double; confirm it still passes and that no new document gate is reachable through it in a way the double doesn't model. If billing's test double bypasses the gate (it calls the gRPC client, not document's handler), the real coupling is only exercised in `e2e` — note that explicitly rather than assuming coverage.

**Coverage:** `document` is in the `others ≥ 75%` tier; record the baseline before starting. `iam` is 87.3%, floor 85%.

## 9. Constraints

- Security change. Senior review; 2 approvals.
- **No Docker, no database.** Never run `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes. Use `pkg/testdb` (skips without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. CI's `Migration Validate` and `Integration Tests (real Postgres)` are the authoritative gates. **This binds delegated subagents.**
- **`Migration Validate` runs against an empty database**, so it validates 021's syntax only — not the grant matrix or idempotency. Those are proven solely by the integration test (the lesson recorded on #168).
- `migrations/` WILL show a diff. `gen/` must stay empty. Do NOT run `make generate-sqlc` (codegen would dirty `services/billing/`, #160).
- **`NewHandler`'s new required param breaks `cmd/document/main.go` — the wiring lands in the same commit as the signature change**, or that commit does not build (the #149 Task 3 lesson).
- Whole-tree `go vet ./...` is the gate. `document.Repository`'s implementers must all be found — `go build` and focused tests miss doubles in other trees.
- **Verification commands containing `$`, `(`, `)` or quotes use `grep -F`.** Count flips with `grep -- '--- FAIL'`, never `grep -E '^\s+--- FAIL'` (matches nothing; reports zero).
- Coverage floors per CLAUDE.md:77 — iam/general-ledger ≥ 85%, budget/expense ≥ 80%, others ≥ 75% (document is `others`).
- **Deploy ordering: migration 021 before the new document code**, or existing tenants lose all thirteen document RPCs. First slice where a fail-closed *and* newly-gated service ships together, so the window is a hard failure, not degraded reads. State prominently in the PR body. (#166 tracks that nothing enforces this.)
- `gh pr checks` before declaring the PR ready.

## 10. Out of scope

- **Gating `billing.DownloadInvoice`** and the rest of billing — slice F.
- **Converting the four log-and-proceed services to fail-closed** — #167.
- **Per-document ownership / ACLs.** This slice gates on tenant-wide role permissions; it does not add "only the uploader may delete". A member holding `document:read` can list every document in the tenant. If finer scoping is wanted, that is a resource-ownership design like the one #157 addressed for tenant isolation — a separate issue.
- **`notifications` (slice G), the #159 audit (slice H), machine tokens (slice I).**
