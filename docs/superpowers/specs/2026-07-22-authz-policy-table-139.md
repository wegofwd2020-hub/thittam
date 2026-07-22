# Authorization policy table — all 127 RPCs (#139)

**Issue:** #139. **Status:** D1, D3, D8, D10 ruled 2026-07-22 — slices A and C are unblocked. D2, D4, D5, D6, D7, D9 remain open and block only their own slices.
**Measured against:** `main` @ `1147f4c` (after #138 authentication, #144 tenant boundary, #146 role-assignment, #149 ledger).

Issue #139's step 1 says the policy table comes first, because "the code has no opinion to read off — this is a decision, not a discovery." This document is that decision, proposed. Every row has a default so the table is complete; the rows that are genuinely contested are collected in §5 and should be ruled on rather than absorbed.

## 1. Current state, measured

| | count |
|---|---|
| Total RPCs (10 services, all unary) | **127** |
| Enforce a permission | 32 |
| Enforce a role (`RequireRole`) | 11 |
| Enforce `user:manage` in-process (iam) | 6 |
| **No authorization check of any kind** | **78** |

#139's header says "~100 ungated". That was true at the time; #144, #146 and #149 have since closed ~22. `services/ledger` is the only fully-gated service.

**Counting caveat for anyone re-auditing:** grepping for `RequirePermission` alone under-counts. `services/iam` gates six RPCs through the helper `h.requireUserManage(ctx)` (`handler.go:237`), which dials nothing — iam answers from its own repository rather than calling itself. A regex that misses that helper reports `AssignRole` as ungated, which #146 fixed.

## 2. The rules the table applies

Seven defaults. Each row below cites the rule that produced it, so a disagreement with a row is usually a disagreement with a rule.

| Rule | Statement |
|---|---|
| **R1** | A read of tenant business data requires `<resource>:read`. |
| **R2** | A write of tenant business data requires `<resource>:write` (or the existing finer verb: `approve`, `submit`, `checkout`, `post`). |
| **R3** | A **vertical-configuration lookup** — categories, templates, labels, phase types, approval limits — requires only an authenticated tenant member. It is configuration, not data; every form in the UI needs it to render, and gating it buys no confidentiality while guaranteeing breakage. |
| **R4** | A **self-scoped** identity call requires only authentication, and takes its subject from the token, never the request body (#149's rule). |
| **R5** | Platform and tenant lifecycle requires `RolePlatformAdmin`. |
| **R6** | Identity and role mutation requires `user:manage`. |
| **R7** | A **service-to-service** RPC requires machine identity, not a user permission. Until machine tokens exist (#139 §4) these stay on the allowlist and are tracked, not gated. |

**R3 is the rule most likely to be wrong.** It trades a small amount of confidentiality (a member learns the tenant's expense categories) for not breaking every form. If you would rather gate those seven RPCs, they collapse into the matching `:read` permission and R3 disappears — see decision D3.

## 3. Vocabulary

**Exists today (18 strings).** Five are declared in `systemRoles` and checked **nowhere** — marked ☠. This table makes all five live, which is why the read slice needs no new vocabulary.

`production:read` ☠, `production:write`, `budget:read` ☠, `budget:write`, `budget:approve`, `expense:submit`, `expense:approve`, `inventory:read` ☠, `inventory:write`, `inventory:checkout`, `inventory:retire` ☠, `report:read` ☠, `resource:manage`, `user:manage`, `ledger:read`, `ledger:write`, `ledger:post`, `ledger:admin`

**Proposed new (8 strings).**

| Permission | Why it cannot reuse an existing string |
|---|---|
| `expense:read` | expense has only `submit` and `approve`. Reading an expense is neither. |
| `billing:read` | No billing vocabulary exists at all. |
| `billing:manage` | Subscription and payment-method mutation is not `write`-shaped; it is account administration. |
| `document:read` | No document vocabulary exists. |
| `document:write` | |
| `document:delete` | Separated from `write` because deletion is the irreversible half — same reasoning that split `ledger:post` from `ledger:write`. |
| `notification:read` | |
| `notification:admin` | Template CRUD is administration, not messaging. |

`inventory:retire` ☠ stays dead: no RPC retires an asset. Either the string or the missing RPC is the defect. Flagged as D7.

## 4. The table

Legend — **AUTH** = any authenticated tenant member; **PUBLIC** = no caller by design; **MACHINE** = service-to-service (R7); ✅ = already enforced, no change; 🔴 = change required.

### 4.1 iam — 30 RPCs (17 enforced, plus `GetCurrentUser` self-scoped by construction)

| RPC | Current | Policy | Rule | |
|---|---|---|---|---|
| `Login` | — | PUBLIC | — | ✅ no caller by design; its `tenant_id` is a directory key, not a claim |
| `RefreshToken` | — | PUBLIC | — | ✅ |
| `ValidateToken` | — | PUBLIC | R7 | ✅ allowlisted; removed when machine tokens land |
| `CheckPermission` | — | MACHINE | R7 | 🔴 today anyone reaching iam's port can probe whether a user holds a permission |
| `AcceptInvitation` | — | PUBLIC | — | ✅ the invitee holds no token; its privileged decision lives upstream at `InviteUser` (#146) |
| `Logout` | — | AUTH | R4 | 🔴 |
| `GetCurrentUser` | — | AUTH | R4 | ✅ **already correct** — `handler.go:83` derives user and tenant from the bearer token and reads no request field |
| `ChangePassword` | — | AUTH, self-only | R4 | 🔴 **see D1 — live defect** |
| `GetUser` | — | `user:manage` OR self | R4/R6 | 🔴 |
| `ListUsers` | — | `user:manage` | R6 | 🔴 |
| `CreateUser` | — | `user:manage` | R6 | 🔴 |
| `UpdateUser` | — | `user:manage` OR self | R6 | 🔴 see D2 |
| `ListRoles` | — | `user:manage` | R6 | 🔴 only an assign-role UI needs it, and that needs `user:manage` anyway |
| `GetTenant` | — | AUTH | R1 | 🔴 |
| `SetTenantAddress` | — | `user:manage` | R6 | 🔴 see D4 |
| `DeactivateUser` | `RolePlatformAdmin` | `user:manage` | R6 | 🔴 see D5 — inconsistent with `AssignRole` |
| `AssignRole` | `user:manage` | unchanged | R6 | ✅ #146 |
| `RevokeRole` | `user:manage` | unchanged | R6 | ✅ #146 |
| `AssignProjectRole` | `user:manage` | unchanged | R6 | ✅ #144 |
| `InviteUser` | `user:manage` | unchanged | R6 | ✅ #146 |
| `CreateTenant` | `RolePlatformAdmin` | unchanged | R5 | ✅ #144 |
| `SuspendTenant` | `RolePlatformAdmin` | unchanged | R5 | ✅ |
| `ClearTenantLegalHold` | `RolePlatformAdmin` | unchanged | R5 | ✅ |
| `SetTenantRetention` | `RolePlatformAdmin` | unchanged | R5 | ✅ |
| `RequestTenantPurge` | `RolePlatformAdmin` | unchanged | R5 | ✅ |
| `ApproveTenantPurge` | `RolePlatformAdmin` | unchanged | R5 | ✅ |
| `CancelTenantPurge` | `RolePlatformAdmin` | unchanged | R5 | ✅ |
| `SetOIDCConfig` | `RolePlatformAdmin` | unchanged | R5 | ✅ |
| `StartImpersonation` | `RolePlatformAdmin` | unchanged | R5 | ✅ gate is right; the **feature** is broken — see D8 |
| `EndImpersonation` | `RolePlatformAdmin` | unchanged | R5 | ✅ |

### 4.2 project — 13 RPCs (7 enforced)

| RPC | Current | Policy | Rule | |
|---|---|---|---|---|
| `CreateProduction` | `production:write` | unchanged | R2 | ✅ |
| `UpdateProduction` | `production:write` | unchanged | R2 | ✅ |
| `ArchiveProduction` | `production:write` | unchanged | R2 | ✅ |
| `CreatePhase` | `production:write` | unchanged | R2 | ✅ |
| `UpdatePhaseStatus` | `production:write` | unchanged | R2 | ✅ |
| `AddCrewMember` | `resource:manage` | unchanged | R2 | ✅ |
| `RemoveCrewMember` | `resource:manage` | unchanged | R2 | ✅ |
| `GetProduction` | — | `production:read` ☠ | R1 | 🔴 |
| `ListProductions` | — | `production:read` ☠ | R1 | 🔴 |
| `ListPhases` | — | `production:read` ☠ | R1 | 🔴 |
| `ListCrewMembers` | — | `production:read` ☠ | R1 | 🔴 |
| `GetEntityLabels` | — | AUTH | R3 | 🔴 vertical config |
| `GetPhaseTypes` | — | AUTH | R3 | 🔴 vertical config |

### 4.3 budget — 13 RPCs (6 enforced)

| RPC | Current | Policy | Rule | |
|---|---|---|---|---|
| `CreateBudget` | `budget:write` | unchanged | R2 | ✅ |
| `SubmitBudget` | `budget:write` | unchanged | R2 | ✅ |
| `ApproveBudget` | `budget:approve` | unchanged | R2 | ✅ |
| `CreateBudgetFromTemplate` | `budget:write` | unchanged | R2 | ✅ |
| `CreateLineItem` | `budget:write` | unchanged | R2 | ✅ |
| `UpdateLineItemActuals` | `budget:write` | unchanged | R2 | ✅ |
| `GetBudget` | — | `budget:read` ☠ | R1 | 🔴 |
| `ListBudgets` | — | `budget:read` ☠ | R1 | 🔴 |
| `GetLineItem` | — | `budget:read` ☠ | R1 | 🔴 |
| `ListLineItems` | — | `budget:read` ☠ | R1 | 🔴 |
| `CheckLineAvailability` | — | `budget:read` ☠ | R1 | 🔴 |
| `GetBudgetCategories` | — | AUTH | R3 | 🔴 vertical config |
| `GetBudgetTemplates` | — | AUTH | R3 | 🔴 vertical config |

### 4.4 expense — 12 RPCs (4 enforced)

| RPC | Current | Policy | Rule | |
|---|---|---|---|---|
| `CreatePurchaseOrder` | `expense:submit` | unchanged | R2 | ✅ |
| `SubmitExpense` | `expense:submit` | unchanged | R2 | ✅ |
| `ApproveExpense` | `expense:approve` | unchanged | R2 | ✅ |
| `CreatePettyCashAdvance` | `expense:submit` | unchanged | R2 | ✅ |
| `GetPurchaseOrder` | — | `expense:read` **new** | R1 | 🔴 |
| `ListPurchaseOrders` | — | `expense:read` **new** | R1 | 🔴 |
| `GetExpense` | — | `expense:read` **new** | R1 | 🔴 |
| `ListExpenses` | — | `expense:read` **new** | R1 | 🔴 |
| `GetPettyCashAdvance` | — | `expense:read` **new** | R1 | 🔴 |
| `ListPettyCashAdvances` | — | `expense:read` **new** | R1 | 🔴 |
| `GetExpenseCategories` | — | AUTH | R3 | 🔴 vertical config |
| `GetApprovalLimits` | — | AUTH | R3 | 🔴 vertical config |

### 4.5 inventory — 7 RPCs (3 enforced)

| RPC | Current | Policy | Rule | |
|---|---|---|---|---|
| `CreateAsset` | `inventory:write` | unchanged | R2 | ✅ |
| `CheckOutAsset` | `inventory:checkout` | unchanged | R2 | ✅ |
| `CheckInAsset` | `inventory:checkout` | unchanged | R2 | ✅ |
| `GetAsset` | — | `inventory:read` ☠ | R1 | 🔴 |
| `ListAssets` | — | `inventory:read` ☠ | R1 | 🔴 |
| `ListCheckouts` | — | `inventory:read` ☠ | R1 | 🔴 |
| `GetInventoryCategories` | — | AUTH | R3 | 🔴 vertical config |

### 4.6 ledger — 12 RPCs (12 enforced) ✅ closed by #149

`CreateAccount` `ledger:write` · `GetAccount` `ledger:read` · `ListAccounts` `ledger:read` · `SeedChartOfAccounts` `ledger:admin` · `OpenAccountingPeriod` `ledger:admin` · `CloseAccountingPeriod` `ledger:admin` · `CreateJournalEntry` `ledger:write` · `PostJournalEntry` `ledger:post` · `GetJournalEntry` `ledger:read` · `ListJournalEntries` `ledger:read` · `VoidJournalEntry` `ledger:post` · `GetTrialBalance` `ledger:read`

### 4.7 billing — 14 RPCs (0 enforced)

| RPC | Policy | Rule | |
|---|---|---|---|
| `GetSubscription` | `billing:read` **new** | R1 | 🔴 |
| `ListInvoices` | `billing:read` **new** | R1 | 🔴 |
| `GetInvoice` | `billing:read` **new** | R1 | 🔴 |
| `DownloadInvoice` | `billing:read` **new** | R1 | 🔴 |
| `ListPaymentMethods` | `billing:read` **new** | R1 | 🔴 |
| `GetUsageSummary` | `billing:read` **new** | R1 | 🔴 |
| `CreateSubscription` | `billing:manage` **new** | R2 | 🔴 |
| `UpgradeSubscription` | `billing:manage` **new** | R2 | 🔴 |
| `CancelSubscription` | `billing:manage` **new** | R2 | 🔴 |
| `AddPaymentMethod` | `billing:manage` **new** | R2 | 🔴 |
| `RemovePaymentMethod` | `billing:manage` **new** | R2 | 🔴 |
| `SetDefaultPaymentMethod` | `billing:manage` **new** | R2 | 🔴 |
| `CheckPlanLimit` | MACHINE | R7 | 🔴 see D6 |
| `HandlePaymentWebhook` | **HMAC signature, not JWT** | — | 🔴 separate issue; not routed today (no `google.api.http` annotation) |

### 4.8 document — 13 RPCs (0 enforced)

| RPC | Policy | Rule | |
|---|---|---|---|
| `GetDocument` | `document:read` **new** | R1 | 🔴 |
| `ListDocuments` | `document:read` **new** | R1 | 🔴 |
| `GetDownloadURL` | `document:read` **new** | R1 | 🔴 a pre-signed URL escapes the tenant boundary once issued |
| `ListVersions` | `document:read` **new** | R1 | 🔴 |
| `ListFolders` | `document:read` **new** | R1 | 🔴 |
| `InitiateUpload` | `document:write` **new** | R2 | 🔴 |
| `ConfirmUpload` | `document:write` **new** | R2 | 🔴 |
| `CreateVersion` | `document:write` **new** | R2 | 🔴 |
| `ConfirmVersion` | `document:write` **new** | R2 | 🔴 |
| `MoveDocument` | `document:write` **new** | R2 | 🔴 |
| `CreateFolder` | `document:write` **new** | R2 | 🔴 |
| `RestoreVersion` | `document:write` **new** | R2 | 🔴 |
| `DeleteDocument` | `document:delete` **new** | R2 | 🔴 |

### 4.9 notifications — 8 RPCs (0 enforced)

| RPC | Policy | Rule | |
|---|---|---|---|
| `Send` | MACHINE | R7 | 🔴 |
| `Dispatch` | MACHINE | R7 | 🔴 |
| `GetNotification` | AUTH, **self-scoped** | R4 | 🔴 see D9 |
| `ListNotifications` | AUTH, **self-scoped** | R4 | 🔴 see D9 |
| `GetTemplate` | `notification:read` **new** | R1 | 🔴 |
| `ListTemplates` | `notification:read` **new** | R1 | 🔴 |
| `CreateTemplate` | `notification:admin` **new** | R2 | 🔴 |
| `UpdateTemplate` | `notification:admin` **new** | R2 | 🔴 |

**Safe to gate:** `cmd/notifications/dispatcher.go` calls `d.svc.Send(...)` — the **service** layer — so the NATS dispatcher does not traverse the handler and is unaffected.

### 4.10 reporting-analytics — 5 RPCs (0 enforced)

| RPC | Policy | Rule | |
|---|---|---|---|
| `GetReportDefinition` | `report:read` ☠ | R1 | 🔴 |
| `ListReportDefinitions` | `report:read` ☠ | R1 | 🔴 |
| `GetExpenseFacts` | `report:read` ☠ | R1 | 🔴 |
| `GetBudgetFacts` | `report:read` ☠ | R1 | 🔴 |
| `GetDashboardSummary` | `report:read` ☠ | R1 | 🔴 |

`report:read` is granted to `super_admin`, `manager`, `coordinator` and `accountant` and checked nowhere. `services/reporting/consumer.go` is NATS-driven against the service layer, so gating the handlers is safe.

## 5. Decisions

**D1, D3, D8 and D10 were ruled on 2026-07-22; the rulings are recorded inline below.** D2, D4, D5, D6, D7 and D9 remain open but block only their own slices, not the sequence.

Each has a recommendation; the recommendation is what the table above already applies.

**D1 — `ChangePassword` is a live defect, not a policy question.** `services/iam/handler.go:216` takes `user_id` from the **request body**, never calls `CallerFromContext`, and `repo.GetUserByID` has no tenant filter. Any authenticated user can change any user's password in any tenant, given the old password. Same defect class as #149's `posted_by`. **RULED 2026-07-22: fix it first, standalone — slice A.** Self-only, subject taken from the token via the #149 pattern. An admin reset, if wanted, is a separate RPC with `user:manage`. This does not wait behind the remaining decisions.

**D2 — may a user update their own profile?** If `UpdateUser` is `user:manage`-only, a user cannot change their own display name without an admin. **Recommend:** `user:manage` OR self, with the self path restricted to non-privileged fields.

**D3 — R3, the seven vertical-config lookups.** AUTH, or fold each into the matching `:read`? **RULED 2026-07-22: AUTH.** Rule R3 stands. They are configuration every form needs; gating them buys no confidentiality and guarantees UI breakage.

**D4 — `SetTenantAddress`: tenant self-service or platform-admin?** It feeds #61's country-driven currency. **Recommend:** `user:manage`, so a tenant admin completes their own onboarding.

**D5 — `DeactivateUser` is `RolePlatformAdmin` while `AssignRole` is `user:manage`.** So a tenant admin can strip a user's roles but not deactivate them, and deactivating requires Thittam staff. **Recommend:** `user:manage`. One of the two is wrong; pick which.

**D6 — `CheckPlanLimit`.** Called to enforce quotas. If only services call it, MACHINE (R7); if the UI shows "you are at 8/10 seats", it also needs `billing:read`. **Recommend:** confirm the caller first — the answer changes the row. (Open.)

**D7 — `inventory:retire` ☠ is granted to `inventory_manager` and no RPC retires an asset.** Either the permission is dead vocabulary to delete, or `RetireAsset` is a missing RPC. **Recommend:** delete the string; add it back with the RPC.

**D8 — impersonation (#139 §5).** `StartImpersonation` writes a session row and an audit entry but mints no token and sets no `act` claim, so subsequent requests carry the admin's own identity. The audit log therefore records something the request path knows nothing about. **RULED 2026-07-22: remove the feature — but NOT by deleting the RPCs.**

`proto/buf.yaml` enables the `FILE` breaking category and CI runs `buf breaking proto --against '.git#branch=main,subdir=proto'`. Removing an RPC from a service is breaking under `FILE`, so deleting `StartImpersonation`/`EndImpersonation` from the proto fails CI.

The harm is the misleading audit trail, not the RPC's existence. So: strip the implementation and the `impersonation_session` write, return `codes.Unimplemented`, and mark both RPCs `// Deprecated:` by comment — the same treatment #144 and #149 gave the fields they retired. The audit log stops recording sessions the request path knows nothing about, the proto stays compatible, and the RPCs can be reclaimed if act-as is ever implemented properly.

**D9 — notification scope.** `ListNotifications` is tenant-scoped but not user-scoped: any member lists every notification in the tenant, contents included. **Recommend:** self-scoped from the token, with a `notification:admin` path for tenant-wide views.

**D10 — do the new permissions need a migration?** `seedSystemRoles` runs only at tenant creation, so new strings reach **new tenants only**. #149 avoided this by inventing no role. Eight new permissions across existing roles need a backfill across every `tenant_<uuid>` schema, or existing tenants silently lose access the moment the gate lands. **This is the single largest hidden cost in #139 and it is not in the issue.**

**RULED 2026-07-22: design the backfill once, in slice D, and reuse it in E/F/G.** Slice D (`expense:read`) is the first slice needing new vocabulary, so it carries the cost of building a per-tenant-schema permission backfill that the later slices reuse. One cross-schema loop to get right instead of four.

## 6. Slice mapping

Ordering follows cost and blast radius, not service order.

| Slice | Scope | RPCs | New vocabulary | Migration |
|---|---|---|---|---|
| **A** | `ChangePassword` actor fix (D1) | 1 | none | no |
| **B** | iam completion — the other 12 rows in §4.1 | 12 | none | no |
| **C** | Read paths — project, budget, inventory, reporting (§4.2, 4.3, 4.5, 4.10) | 19 | **none** — revives 4 ☠ strings | no |
| **D** | expense reads | 8 | `expense:read` | **yes** |
| **E** | document | 13 | `document:read/write/delete` | **yes** |
| **F** | billing | 12 (+2 deferred) | `billing:read/manage` | **yes** |
| **G** | notifications | 8 | `notification:read/admin` | **yes** |
| **H** | Prove tenant isolation on reads (#139 §3) | 0 — tests only | none | no |
| **I** | Machine tokens; drop the two allowlist entries (#139 §4) | 0 | none | no |

**A and C first.** A is a live defect. C closes 19 of 78 with no new vocabulary, no migration and no proto change — and it is the cheapest way to find out whether the grant matrix is right before inventing eight more strings. D10 (the backfill) blocks D through G and should be designed once, in D, then reused.

## 7. What this table deliberately does not decide

- **Project-scoped RBAC.** `RequirePermission` derives a `projectID` from `caller.ProjectID` behind `ProjectScopedRBACEnabled()`. Every row above is tenant-scoped. Per-project grants are #42.
- **A subset rule.** A `super_admin` may still grant any permission they do not hold. Unnecessary while no RPC mints a role; wrong once custom roles exist.
- **`GetUserPermissions`' missing `tenant_id` filter.** Correct today only because `user_roles` reference own-tenant roles — an invariant #146 enforces at the write path while the read path assumes it. Belongs with slice H.
