# Ledger authorization and actor integrity — design

**Issue:** #149, slice 3 of #139.
**Follows:** #138 (authentication), #144 (tenant boundary), #146 (role-assignment authorization).
**Branch:** `feat/ledger-authz-149`, base `785fbeb`.

## 1. The problem

`services/ledger/handler.go` exposes 12 RPCs. Every one is guarded by `interceptor.TenantFromRequest` and nothing else. That is a *tenant* check, not a *privilege* check: it proves the caller belongs to the tenant whose books they are about to modify. It does not ask whether they may modify them.

So any authenticated member of a tenant — a `member` role holding only `production:read` and `expense:submit` — can today create accounts, draft journal entries, post them, void them, seed the chart of accounts, and open and close accounting periods.

`Handler` has no `perm` field. `cmd/general-ledger/main.go` never calls `iamclient.DialFromEnv`. The service has no permission checker at all, so this is not a gap in coverage; it is a total absence.

### 1.1 A second, independent defect: actor forgery

Three RPCs take the acting user's id from the **request body**:

| RPC | `handler.go` | reads |
|---|---|---|
| `CloseAccountingPeriod` | `:165` | `req.GetClosedBy()` |
| `PostJournalEntry` | `:247` | `req.GetPostedBy()` |
| `VoidJournalEntry` | `:315` | `req.GetVoidedBy()` |

A permission gate does not fix this. A caller holding `ledger:post` could still attribute a posting to any other user, including a user in the same tenant who never touched the ledger. In double-entry accounting the actor is half the record — `posted_by` is what the audit trail exists to capture.

This is the same forgery class #144 fixed in `AssignProjectRole`. It survived there because #144 scoped itself to `tenant_id`.

`CreateJournalEntry` has no `created_by` field and needs no actor fix. There are three fields, not four.

## 2. Vocabulary: four permissions

No `ledger:*` permission exists anywhere in `systemRoles`. The ledger is the first service that must invent vocabulary rather than reuse it — project, budget, inventory and expense all gated on strings that already existed.

The twelve RPCs cluster into four duties, and the clustering is not arbitrary. Accounting has a structural control called **separation of duties**: the person who drafts a journal entry should not be the person who commits it, and neither should be the one who closes the period. A single `ledger:write` would collapse three duties into one grant, and no later gate could pull them apart without re-gating RPCs and re-granting roles.

| Permission | RPCs |
|---|---|
| `ledger:read` | `GetAccount`, `ListAccounts`, `GetJournalEntry`, `ListJournalEntries`, `GetTrialBalance` |
| `ledger:write` | `CreateAccount`, `CreateJournalEntry` |
| `ledger:post` | `PostJournalEntry`, `VoidJournalEntry` |
| `ledger:admin` | `SeedChartOfAccounts`, `OpenAccountingPeriod`, `CloseAccountingPeriod` |

### 2.1 Grants

| Role | `ledger:read` | `ledger:write` | `ledger:post` | `ledger:admin` |
|---|:-:|:-:|:-:|:-:|
| `super_admin` | ✓ | ✓ | ✓ | ✓ |
| `accountant` | ✓ | ✓ | ✓ | |
| `manager` | ✓ | | | |
| `coordinator` | ✓ | | | |

`project_supervisor`, `member` and `inventory_manager` receive none.

`accountant` holds `post` because a bookkeeper who may draft an entry but never commit it is not a bookkeeper. `ledger:admin` — closing a period, seeding the chart — stays with `super_admin`, so ending an accounting period remains a separately-privileged, deliberate act.

The alternative considered and rejected: give `accountant` only read and write, leaving `super_admin` as the sole poster. That is the strictest reading of separation of duties, but with no middle role every posting in every tenant runs as the same identity that holds `user:manage`. Teams respond by handing everyone `super_admin`, which is strictly worse than the status quo. A dedicated `controller` role would be cleaner still, but `seedSystemRoles` runs only at tenant creation and #146 established that no RPC can mint a role — a new role needs a backfill migration across every `tenant_<uuid>` schema. Out of scope here.

### 2.2 Constants, not literals

`permUserManage` (`services/iam/service.go:49`) is currently the only permission promoted to a named constant; every other string in `systemRoles` is an inline literal. The four new permissions follow `permUserManage`'s style:

```go
// Ledger permissions encode separation of duties: drafting an entry (ledger:write),
// committing it (ledger:post), and controlling the books (ledger:admin) are three
// distinct grants. Collapsing them would remove the control an auditor looks for.
const (
	permLedgerRead  = "ledger:read"
	permLedgerWrite = "ledger:write"
	permLedgerPost  = "ledger:post"
	permLedgerAdmin = "ledger:admin"
)
```

A typo in a permission string fails **open** at the grant site (a role holding `"ledger:pots"` simply never matches) and fails **closed** at the check site. Constants make both impossible.

The ledger handler cannot import these from `services/iam` — the handler passes a string literal to `RequirePermission`, and iam's package is not a dependency of ledger. The constants live in iam because that is where `systemRoles` lives; `services/ledger/handler.go` declares its own identical constants. This duplication is deliberate and small; a shared `pkg/perm` is the right answer once a second service needs the same strings, and that is not today.

## 3. `ActorFromRequest`

New file `pkg/interceptor/actor.go`. It mirrors `TenantFromRequest` (`pkg/interceptor/tenant.go`) deliberately, down to the failure modes.

```go
// ActorFromRequest returns the caller's user id, taken from the verified token.
//
// If the request also names an actor and it differs, the call is refused. A client
// asking to act as somebody else is either broken or hostile; either way we would
// rather say so than quietly record the wrong name against a journal entry.
//
// The returned id NEVER comes from the request. A handler cannot attribute a posting
// to a caller-supplied user, because the only id it can obtain is this one.
func ActorFromRequest(ctx context.Context, reqActorID string) (uuid.UUID, error) {
	caller, ok := CallerFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if caller.UserID == uuid.Nil {
		return uuid.Nil, status.Error(codes.Unauthenticated, "token carries no subject")
	}
	if reqActorID == "" {
		return caller.UserID, nil
	}
	reqID, err := uuid.Parse(reqActorID)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid actor id")
	}
	if reqID != caller.UserID {
		// Deliberately does not echo either id: the caller already knows its own,
		// and confirming the existence of the other is an oracle.
		return uuid.Nil, status.Error(codes.PermissionDenied, "actor does not match the authenticated caller")
	}
	return caller.UserID, nil
}
```

Three properties are load-bearing.

**It returns the trusted value, never the validated one.** Even in the branch where `reqID == caller.UserID`, it returns `caller.UserID`. Identical today; but a future edit that loosens the comparison then cannot leak caller-controlled data into the audit trail.

**It returns `uuid.Nil` on every error path.** A handler that ignores the error attributes the entry to a user id that matches nobody, rather than to a plausible one.

**It cannot be skipped.** It returns the value the handler must pass to the service. Delete the call and there is nothing to give `h.svc.PostJournalEntry`. This is the property `RequirePermission` lacks — an `error`-only guard was omitted at twenty call sites for about a year, behind `if h.perm != nil`, and nothing broke because nothing was checked.

An empty `reqActorID` is accepted and resolves to the caller. That is the path a well-behaved client takes once the field is deprecated, and it is why deprecating rather than removing the field is safe.

## 4. Handler changes

`Handler` gains a checker; `NewHandler` takes it as a **required parameter**:

```go
type Handler struct {
	ledgerv1.UnimplementedLedgerServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}
```

This departs from the house idiom. The other four gated services use `NewHandler(svc)` plus an optional `WithPermissionChecker(p)` setter, called only when the dial succeeded. That idiom is *safe* — `cmd/project-management/main.go:90` guards it with `if iamPerm != nil`, so the concrete nil never crosses into the interface field and `RequirePermission`'s nil branch fires correctly (`services/budget/handler_test.go:459` regression-tests exactly this). It is not, however, *enforced*: a future `NewHandler(svc)` with no setter call compiles cleanly and yields a handler whose every RPC returns `Internal`.

A required parameter makes forgetting the checker a build error. The cost is that all 34 handler-construction sites in `services/ledger/handler_test.go` must be updated, and because Go compiles per package, the 34 `Service`-level tests in `services/ledger/service_test.go` will not build until they are. That cost is paid once, in this branch.

### 4.1 Guard order

Every gated RPC runs the same sequence:

```go
func (h *Handler) PostJournalEntry(ctx context.Context, req *ledgerv1.PostJournalEntryRequest) (*ledgerv1.JournalEntry, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, permLedgerPost); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid journal entry ID")
	}
	postedBy, err := interceptor.ActorFromRequest(ctx, req.GetPostedBy())
	if err != nil {
		return nil, err
	}
	je, err := h.svc.PostJournalEntry(ctx, tenantID, id, postedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return journalEntryToProto(je), nil
}
```

**Tenant, then permission, then parses, then actor.** This matches `services/budget/handler.go:160` (`ApproveBudget`), which gates before parsing.

The ordering has consequences worth stating. An unauthorized caller receives `PermissionDenied` regardless of whether their `id` was well-formed — the parse error is not an oracle for them. Conversely, this means an *authorized* caller with a malformed id gets `InvalidArgument`, and an unauthorized caller with the same malformed id gets `PermissionDenied`. That asymmetry is intended.

Actor resolution comes last because forging an actor you were never permitted to use is the less interesting denial; the caller is already refused.

#146 shipped a defect in exactly this area: a permission gate placed ahead of an *optional* UUID parse converted an `InvalidArgument` into a `PermissionDenied` and silently changed a test's meaning. Here the parse of `id` is required, sits after the gate by design, and its test must therefore grant the permission. Every existing ledger test that asserts `InvalidArgument` must be given an allowing checker, and its assertion must not change.

### 4.2 The full mapping

| RPC | `handler.go` | Permission | Actor |
|---|---|---|---|
| `CreateAccount` | `:40` | `permLedgerWrite` | — |
| `GetAccount` | `:68` | `permLedgerRead` | — |
| `ListAccounts` | `:86` | `permLedgerRead` | — |
| `SeedChartOfAccounts` | `:106` | `permLedgerAdmin` | — |
| `OpenAccountingPeriod` | `:139` | `permLedgerAdmin` | — |
| `CloseAccountingPeriod` | `:154` | `permLedgerAdmin` | `ActorFromRequest(ctx, req.GetClosedBy())` |
| `CreateJournalEntry` | `:179` | `permLedgerWrite` | — |
| `PostJournalEntry` | `:236` | `permLedgerPost` | `ActorFromRequest(ctx, req.GetPostedBy())` |
| `GetJournalEntry` | `:259` | `permLedgerRead` | — |
| `ListJournalEntries` | `:277` | `permLedgerRead` | — |
| `VoidJournalEntry` | `:304` | `permLedgerPost` | `ActorFromRequest(ctx, req.GetVoidedBy())` |
| `GetTrialBalance` | `:329` | `permLedgerRead` | — |

## 5. Startup

`cmd/general-ledger/main.go` today has no IAM wiring whatsoever. It gains the dial, and refuses to start without a checker:

```go
// The ledger cannot authorize without IAM. #138's convention for the JWT public key
// applies here too: a service that cannot enforce its guarantees does not start.
// Starting would serve codes.Internal on every accounting RPC, which reads as a bug
// rather than a misconfiguration.
iamPerm, closeIAM, err := iamclient.DialFromEnv("general-ledger")
if err != nil {
	log.Fatalf("general-ledger: startup: dial IAM: %v", err)
}
defer func() { _ = closeIAM() }()
if iamPerm == nil {
	log.Fatalf("general-ledger: startup: %s is not set; the ledger cannot authorize without a permission checker", iamclient.EnvAddr)
}
handler := ledger.NewHandler(svc, iamPerm)
```

`iamPerm` is the concrete `*iamclient.PermissionChecker` at the point of the nil check, so this is a plain pointer comparison, not the typed-nil trap.

This diverges from the four existing services, which log and proceed. Converting them is a separate change with a five-service deploy-ordering hazard; this spec touches only the ledger.

**No infrastructure change is required.** `IAM_SERVICE_ADDR` is already set globally in `infra/k8s/config/configmap.yaml:78` and reaches every pod through `envFrom: configMapRef: thittam-common` — general-ledger's Deployment (`infra/k8s/services/general-ledger.yaml:58`) has the identical env block to budget-planning's. The NetworkPolicy already permits ledger→iam:8086.

## 6. Proto

The three actor fields are deprecated by comment, matching the convention #144 established on `tenant_id` in these same messages. `grep -rn "deprecated = true" proto/` returns nothing tree-wide; the codebase does not use the proto field option.

```protobuf
message PostJournalEntryRequest {
  // Deprecated: ignored. The tenant is derived from the caller's verified token
  // (#144). Sending a value that differs from the token's tenant is rejected
  // with PermissionDenied.
  string tenant_id = 1;
  string id = 2;
  // Deprecated: ignored. The actor is derived from the caller's verified token
  // (#149). Sending a value that differs from the authenticated caller is rejected
  // with PermissionDenied.
  string posted_by = 3;
}
```

Same paragraph on `CloseAccountingPeriodRequest.closed_by` and `VoidJournalEntryRequest.voided_by`.

Fields are **not** removed. `proto/buf.yaml` enables the `FILE` breaking category, and CI runs `buf breaking proto --against '.git#branch=main,subdir=proto'`. A comment changes neither number, name, nor type, so the job passes. Removal would require a deliberate break and a `reserved` field number; deprecation costs nothing and `ActorFromRequest` accepts the empty string, so a client that stops sending the field works immediately.

The entity messages `AccountingPeriod.closed_by` (`:66`) and `JournalEntry.posted_by` (`:82`) are **untouched** — those are the stored record, not caller input.

## 7. Testing

`services/ledger/handler_test.go` already has `callerCtx(tid uuid.UUID)`, which injects a `CallerInfo` via `interceptor.WithCaller`. Every existing test uses it, so no test lacks a caller. The breakage is entirely from `NewHandler`'s new parameter and from the gates.

There is no shared mock `PermissionChecker`; each of the four gated services declares its own unexported `allowAllPerm`. The ledger follows that convention and adds a denying counterpart:

```go
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm denies everything, so a test can prove a gate fires before any repository call.
type denyPerm struct{}

func (denyPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return false, nil
}
```

### 7.1 What the new tests must prove

**Each gate fires before the repository is reached.** `mockRepo` uses function fields. An unset field returns a benign zero value and never panics, so a denial test that merely asserts `PermissionDenied` would pass against an ungated handler that wrote first and denied second. Every denial test therefore installs the relevant repo fn with a body that calls `t.Fatal`. This is the trap #146 documented; it applies verbatim.

**Actor forgery is rejected, not silently corrected.** `PostJournalEntry` with `posted_by` set to a different user than the caller returns `PermissionDenied` and never reaches `updateJournalStatusFn`.

**The actor actually written is the caller.** `PostJournalEntry` with an empty `posted_by` must call `Service.PostJournalEntry` with `caller.UserID`. `Service` takes `(ctx, tenantID, id, postedBy)` — three consecutive `uuid.UUID` — so a transposition compiles silently, and `mockRepo`'s fn-fields ignore arguments they are not asked about. `Repository.UpdateJournalStatus(ctx, id, status, actorID, at)` receives both the entry id and the actor, so the test captures them from `updateJournalStatusFn` and asserts `actorID == caller.UserID` and `actorID != id`. The caller's id, the entry id and the tenant id are distinct fixtures, so an `id`↔`postedBy` transposition turns the test red. #146 shipped this hazard unguarded and caught it only in the whole-branch review.

**Each denial test fails against the pre-change handler.** Verified by checking out the parent commit into a scratch `git worktree` under `/tmp`, dropping in the new test file, and running it. A denial test that passes against ungated code is a tautology.

### 7.2 Existing tests

All 34 construction sites take `allowAllPerm{}`. No existing assertion changes. Any test whose assertion must change is a signal that the guard order is wrong — send it back rather than bending it.

`e2e/critical_path` is unaffected: it constructs `ledger.NewService(repo)` and calls `svc.PostJournalEntry` directly (`critical_path_test.go:243`), never touching the handler or the interceptor chain. It certifies nothing about this change, which is consistent with #138 and #144.

## 8. Error handling

`grpcErr` (`handler.go:432`) maps domain sentinels and needs no change. `RequirePermission` and `ActorFromRequest` both return `*status.Status` errors already, returned directly and never passed through `grpcErr` — the same discipline `TenantFromRequest` uses today.

`RequirePermission` returns `codes.Internal` when the checker is nil or the iam call fails. The nil case is now unreachable in production, because `cmd/general-ledger` refuses to start; it remains reachable in tests and stays as defense in depth.

Neither guard echoes the caller's permissions, roles, or user id. `RequirePermission`'s `"missing required permission: %s"` names the permission required, never the permissions held — that distinction is what keeps it from being an authorization oracle.

## 9. What this does not fix

**`Repository.UpdateJournalStatus` takes no `tenantID`.** `services/ledger/db/postgres.go:300` compensates with `resolveTenantForJournal`, an unscoped `SELECT tenant_id FROM journal_entries WHERE id = $1` whose error is discarded — on failure it returns `uuid.Nil` and the update silently matches nothing. Not exploitable today: `Service.PostJournalEntry` and `Service.VoidJournalEntry` both call the tenant-scoped `GetJournalEntry(ctx, tenantID, id)` first, so a wrong-tenant id returns `ErrJournalNotFound` before `UpdateJournalStatus` runs. It is an invariant held by call-site discipline rather than by types — the shape that rots. Separate issue.

**Roughly 90 RPCs across the platform still enforce nothing**, `reporting-analytics` has no permission check at all, and five permission strings (`production:read`, `budget:read`, `report:read`, `inventory:read`, `inventory:retire`) are declared in roles and checked nowhere. All remain in #139.

**The four services that log-and-proceed on a nil checker** keep doing so. Their `// optional; nil skips RequirePermission` field comments (`services/project/handler.go:22` and three siblings) have been false since #138 — `RequirePermission` denies with `Internal`, it does not skip. Correcting four comments is in scope for this branch; changing their startup behaviour is not.

**No subset rule.** A `super_admin` may grant `ledger:admin` to anyone. Unnecessary while the seven system roles are the whole universe and no RPC mints a role; the wrong shape once custom roles exist. #139.

## 10. Global constraints

- Security change: senior review, two approvals (`CLAUDE.md`).
- Monetary values stay `decimal.Decimal` / `NUMERIC(14,2)`. This change touches no amounts.
- Whole-tree `go vet ./...` is the gate.
- `errcheck` runs in CI; `golangci-lint` is not installed locally.
- Coverage: `services/ledger` and `pkg/interceptor` must not regress. Threshold for `others` is ≥ 75%.
- `gh pr checks` before declaring the PR ready. Local green is not CI green.
- No Docker, no database. Never `docker compose … -v` / `down` / `up` against `infra/local/` — it is project-scoped and `-v` deletes all local volumes. A disposable `git worktree` under `/tmp` is fine; remove it afterwards.
- No migration. No proto field added or removed. `git diff --stat gen/` must be empty.
