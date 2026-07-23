# iam completion — permission gates, a cross-tenant read, and a status wipe — design

**Issue:** #139, slice B. **Branch:** `fix/iam-completion-139b`, base `e1871c5` (`main`).
**Follows:** #138 (authentication), #144 (tenant boundary), #146 (role-assignment), #149 (ledger + `ActorFromRequest`), #139 slice A (`ChangePassword`), #139 slice C (read-path gates), #157 (tenant isolation, PR #161).
**Policy table:** `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`.

## 1. What this slice is, and what it turned out to be

Slice B was scoped as "gate the remaining ungated iam RPCs". Grounding found the gating half is small — the reads are already tenant-bounded and need no change at all — and that two **live defects** sit inside the same handlers. Those dominate the value of the slice.

| defect | severity |
|---|---|
| `GetTenant` reads any tenant's metadata by UUID, with no tenant derivation and no caller check | **cross-tenant read** |
| Ungated `UpdateUser` writes `status` unconditionally, undoing `DeactivateUser` | **privilege escalation** |

## 2. The gating decision — no new vocabulary, no backfill

`user:manage` is the **only** user- or tenant-related permission string in the codebase. There is no `user:read`, no `tenant:read`. It is held by `super_admin` alone, out of seven seeded roles (`systemRoles`, `services/iam/service.go:66`).

So gating the read RPCs on the vocabulary that exists would stop a `manager` listing their own team, a `coordinator` looking up a colleague to assign, and every non-admin role reading its own tenant's name or currency. Introducing `user:read` / `tenant:read` instead would mean editing `systemRoles`, which runs only at tenant creation — reaching **new tenants only** and requiring the D10 cross-schema backfill.

**Ruling taken: reads are AUTH, writes require `user:manage`.** This applies decision D3, already settled for vertical config: within a tenant, the user directory is not privileged information, and the tenant boundary is the real control. It invents no string, edits no role, and needs **no backfill** — the same property that let slice C ship first.

| RPC | policy | change needed |
|---|---|---|
| `GetUser`, `ListUsers`, `ListRoles` | AUTH | **none** — already `TenantFromRequest`-bounded |
| `Logout` | AUTH | **none** — not on `PublicMethods`, so a valid token is already required |
| `GetTenant` | AUTH | **fix the cross-tenant read** (§3) |
| `CreateUser`, `UpdateUser`, `SetTenantAddress` | `user:manage` | add `h.requireUserManage(ctx)` |
| `DeactivateUser` | unchanged | `RequireRole(platform_admin)` retained — see §5 |

### 2.1 Why the reads need no code change

After #144, `interceptor.TenantFromRequest(ctx, req.GetTenantId())` derives the tenant from the caller's verified token and rejects a mismatched request tenant with `PermissionDenied`. `GetUser`, `ListUsers` and `ListRoles` all call it as their first statement. Authentication is enforced by `UnaryAuthInterceptor` for every method not on `PublicMethods`. "AUTH, tenant-scoped" is therefore already their behaviour; recording the decision is the whole of the work.

Stating this explicitly matters: a reviewer expecting eight code changes should know that five of them are deliberate no-ops, not omissions.

### 2.2 iam must not use `interceptor.RequirePermission`

iam gates in-process via `h.requireUserManage(ctx)` (`services/iam/handler.go:241`), which calls `h.svc.CheckPermission` against its own repository. The interceptor path would make iam dial itself, and after #138 a nil checker makes it return `Internal` for every gated RPC. The helper already fails closed on all three paths — no caller, lookup error, negative answer. Four RPCs use it today (`AssignRole`, `RevokeRole`, `AssignProjectRole`, `InviteUser`); this slice adds three more.

**Guard order differs between the two groups, deliberately.** In the four pre-existing RPCs the `requireUserManage` call comes *after* the `uuid.Parse` calls on the path's ids (`AssignRole` ~263-282, `RevokeRole` ~290-307, `AssignProjectRole` ~350-373, `InviteUser` ~575-594). The three new gates (`CreateUser`, `UpdateUser`, `SetTenantAddress`) put `requireUserManage` *before* their `uuid.Parse` calls. The new order is the better one — it denies an unauthorized caller without first telling them whether their argument was well-formed — and it is not being changed to match the old four. Aligning the older four to gate-before-parse is a possible follow-up, not done in this slice.

## 3. `GetTenant` — a cross-tenant read

```go
func (h *Handler) GetTenant(ctx context.Context, req *iamv1.GetTenantRequest) (*iamv1.Tenant, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	tenant, err := h.svc.GetTenant(ctx, id)
	...
```

No `TenantFromRequest`, no `CallerFromContext`. **Any authenticated user can read any tenant's record by UUID** — name, slug, plan, status, full billing address, country, currency, and the `#92` lifecycle timestamps (`suspended_at`, `deactivated_at`, retention state).

This is the #144 defect class, and it survived #144 for a mechanical reason: #144 scoped itself to fields named `tenant_id`, and this message's field is named `id`. The field *is* the tenant id; only the name differs.

**Fix:** derive the tenant the same way every other tenant-scoped RPC does.

```go
tenantID, err := interceptor.TenantFromRequest(ctx, req.GetId())
if err != nil {
	return nil, err
}
tenant, err := h.svc.GetTenant(ctx, tenantID)
```

`TenantFromRequest` returns `caller.TenantID` when the request field is empty and `PermissionDenied` when it differs — so a client asking for its own tenant keeps working, and a client asking for another's is refused rather than silently redirected. It also returns the value the handler must pass on, so the guard cannot be skipped (`feedback-enforce-guards-by-type`).

**No internal caller breaks.** Every other `GetTenant` call site is service- or repository-layer and bypasses the handler entirely: `billing_consumer.go:53`, `purge.go:30,77`, `lifecycle.go:84`, `retention_override.go:39`, `service.go:246,550,582,593,634`. Only `handler.go:428` is the RPC.

**Platform-admin cross-tenant reads are not supported by this RPC today and are not added here.** Nothing in the tree calls `GetTenant` for a tenant other than the caller's. If a platform-admin console later needs one, that is a separate RPC with a `RequireRole` gate, not a hole in this one.

## 4. `UpdateUser` — the status wipe

```go
// services/iam/db/postgres.go
const q = `UPDATE users SET display_name = $2, status = $3 WHERE id = $1 AND tenant_id = $4`
```

`Handler.UpdateUser` builds `User{DisplayName: req.GetDisplayName(), Status: req.GetStatus()}` and `Service.UpdateUser` passes it straight through. There is no gate, no validation of `status`, and no `COALESCE`.

`status` is security-critical: `pkg/auth/local.go:84-89` refuses login for `deactivated` and `invited` accounts.

**Correction (post-review):** an earlier draft of this section claimed that `status: ""` wiped the column to the empty string and that the login switch then matched neither `deactivated` nor `invited`, letting a deactivated account log in again through an ordinary profile edit. **That chain is unreachable and the claim was wrong.** `migrations/iam/002_create_users.up.sql:11-12` declares:

```sql
status        TEXT        NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active', 'invited', 'deactivated')),
```

and no `ALTER TABLE users` or `DROP CONSTRAINT users_status_check` exists anywhere in `migrations/`, `seeds/` or `infra/`. The pre-fix `UPDATE users SET display_name = $2, status = $3 ...` with `$3 = ''` therefore raised Postgres error `23514 check_violation` — it never wrote an empty string, and the login-switch scenario never happened.

**What the pre-fix code really did:** every `UpdateUser` call that changed only a display name sent `status: ""` and **failed with a constraint violation**, because the old SQL wrote `$3` unconditionally. That is a real functional break — any legitimate display-name edit errored out — and `COALESCE(NULLIF($3, ''), status)` is the correct fix for it. It is also defence-in-depth if the CHECK constraint were ever dropped or the value set changed elsewhere.

**Where the genuine privilege escalation is:** the *ungated* `UpdateUser` let any authenticated member send `status: "active"` — a CHECK-legal value — on a deactivated colleague, undoing `DeactivateUser`, which is gated on `platform_admin`, the strictest control in the service. That escalation is closed by the `user:manage` gate, not by the `NULLIF` change; `NULLIF` alone would still let a gate-holder or (pre-gate) any member flip a colleague back to `active`.

`pkg/auth/local.go:84-89` is a default-allow switch — it rejects only `deactivated` and `invited`, so any value outside that pair falls through to login-allowed. It is the CHECK constraint, not the login switch, that actually prevents an out-of-set value from ever reaching it: the switch's default-allow shape only matters for values Postgres would accept, and `active`/`invited`/`deactivated` is the whole set.

The integration test for this has teeth in both worlds: before the fix it fails at `require.NoError` on the constraint violation from a display-name-only update; after the fix, that same call passes and the stored status is unchanged.

This is the #146 shape: a strict gate rendered decorative by an ungated sibling writing the same column.

**Gating alone does not fix the constraint-violation break.** A legitimate `user:manage` holder editing a display name would still hit the CHECK violation. Both halves are needed:

- Add `h.requireUserManage(ctx)` to the handler.
- Make the write preserve an unspecified status:

```sql
UPDATE users SET display_name = $2, status = COALESCE(NULLIF($3, ''), status)
WHERE id = $1 AND tenant_id = $4
```

An empty `status` now means "leave it alone" rather than "clear it". A caller that genuinely wants to change status still sends a value.

**Why not validate `status` against an allow-list instead?** That is the better long-term shape, but it is a behaviour change for any client currently sending an unrecognised value, and the set of legal statuses is not declared in one place today (`active`, `invited`, `deactivated` appear as string literals across `service.go`, `local.go` and the retention code). Introducing an enum is worth doing and is out of scope here; `NULLIF` closes the live hole without guessing at the vocabulary.

## 5. `DeactivateUser` — inconsistency recorded, not changed

It is gated on `RequireRole(platform_admin)` while its neighbours use `user:manage` (open decision D5). Deactivation is destructive, and `platform_admin` is the stricter control; changing it to `user:manage` would **widen** who may deactivate users, from platform admins to every per-tenant `super_admin`. Widening access is the wrong direction to move on a security branch without a product reason. The inconsistency is recorded in the policy table and left alone.

Note that §4 is what actually makes this gate meaningful: today the strict gate is undone by an ungated sibling.

## 6. What does not change

No migration. No proto change — no field added, removed or renumbered, so `buf breaking` is unaffected and `git diff --stat gen/` must be empty. No new permission string, no `systemRoles` edit, **no D10 backfill**. No change to the five RPCs in §2.1. No service-layer or repository-layer signature changes except the `UpdateUser` SQL text.

## 7. Testing

**Denial tests** for the three newly gated RPCs, each proving the gate fires before the repository is reached. `services/iam`'s `mockRepo` has an **inverted trap** relative to the other services: its unset `getUserFn`/`getRoleByIDFn` return objects *in the caller's tenant*, so an ownership test that forgets to stub them passes vacuously. Unset write-fns return `nil` and never panic. Therefore "the repository was never reached" is proven **only** by a write-fn whose body calls `t.Fatal`.

**The denial-test rule, earned in slice A:** a denial test must trip on the first repository call its path should never reach, and the status code it asserts must not be reachable by another route. `grpcError` maps several errors onto the same codes; assert on the tripwire, not the code alone.

**`GetTenant` cross-tenant test:** caller in tenant A requests tenant B's id, expect `PermissionDenied` from `TenantFromRequest`, and assert `getTenantFn` was never called. A same-tenant request and an empty-id request must both still succeed.

**Status-wipe regression test:** call `UpdateUser` with `Status: ""` against a user whose stored status is `deactivated`, and assert the value written is still `deactivated`. This is the test that would have caught the live defect, and it must fail against the current SQL.

**Existing tests that will flip — predicted: exactly 5.**

`mockRepo.GetUserPermissions` returns `nil, nil` when unstubbed (`service_test.go:282`), so `Service.CheckPermission` answers `false` and `requireUserManage` returns `PermissionDenied`. Nine tests call the three RPCs, all via `memberCtx(tid)`, but only those that pass a **valid** tenant reach the new gate:

| test | after | why |
|---|---|---|
| `TestHandler_CreateUser_Success` | flips | reaches the gate |
| `TestHandler_UpdateUser_Success` | flips | reaches the gate |
| `TestHandler_UpdateUser_InvalidID` | flips | gate precedes the `uuid.Parse` |
| `TestHandler_SetTenantAddress_Success` | flips | reaches the gate |
| `TestHandler_SetTenantAddress_MissingCountry` | flips | reaches the gate |
| `TestHandler_CreateUser_InvalidTenantID` | unchanged | fails at `TenantFromRequest` first |
| `TestHandler_UpdateUser_InvalidTenantID` | unchanged | same |
| `TestHandler_SetTenantAddress_InvalidTenantID` | unchanged | same |
| `TestCreateUser_CrossTenant_Denied` | unchanged | cross-tenant, refused by `TenantFromRequest` |

Repair by stubbing `getUserPermissionsFn` to return `[]string{"user:manage"}` — never by weakening an assertion. `_InvalidID` must still assert `InvalidArgument`, which it reaches once the caller holds the permission.

**If the actual count is not 5, stop and report.** Slice C predicted 3 and got 5; the discrepancy was benign but was only known to be benign because it was investigated rather than absorbed.

**Guard order** for the three new gates is tenant → permission → parse. This does **not** match the four RPCs that already use `requireUserManage` (`AssignRole`, `RevokeRole`, `AssignProjectRole`, `InviteUser`), which parse their ids *before* checking the permission (see §2.2) — the new order is deliberately different and better, denying an unauthorized caller before revealing whether their argument was well-formed. Aligning the older four is a possible follow-up, out of scope here. The tenant → permission → parse order on the new three is what keeps the three `_InvalidTenantID` tests unchanged, and it is why `_InvalidID` flips.

**Coverage** on `services/iam` must not regress. Baseline **87.2%**; the tier floor for iam is 85%.

## 8. Constraints

- Security change touching `iam`. Senior review; 2 approvals.
- **No Docker, no database.** Never run `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it destroyed unrelated MinIO dev data once. Use `pkg/testdb` (integration tests SKIP without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. CI's real-Postgres job is the authoritative gate. **This binds delegated subagents — state it in their instructions.**
- Whole-tree `go vet ./...` is the gate. `iam.Repository` has **three** implementers including a hidden e2e double `iamRepo` in `e2e/critical_path/helpers_test.go`; `go build` and focused tests both miss it.
- `make generate-sqlc` is repo-wide and dirties `services/billing/` with pre-existing drift (#160). Revert it before committing; never `git add -A`.
- `errcheck` runs in CI; `golangci-lint` is not installed locally.
- `gofmt -l services/iam` flags `service.go` and `lifecycle_test.go` on a clean `main`. Pre-existing; CI's Lint does not gate on gofmt. Do not reformat them.
- `gh pr checks` before declaring the PR ready.

## 9. Out of scope

- **Self-service profile editing (decision D2).** `UpdateUser` sets a self-service field (`display_name`) and a security-critical one (`status`) in the same call, so a "the caller may edit themselves" carve-out would let a user set their own status. Closing it properly needs a separate `UpdateProfile` RPC that cannot touch `status`. **File as a new issue.**
- **A `status` enum / allow-list validation.** §4 explains why `NULLIF` is the right fix now.
- **`CheckPermission` on `PublicMethods`.** Unchanged; #139 §4 / slice I.
- **The remaining #139 slices** — D (expense reads + the D10 backfill), E/F/G (document, billing, notifications), H (tenant-isolation audit, #159), I (machine tokens).
- **`RevokeRole`'s missing tenant scope** and role-revocation staleness (#154). Untouched.
