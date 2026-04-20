# Multi-Tenancy in Thittam

**Status:** Living document
**Last updated:** 2026-04-15
**Audience:** Developers, operators, and anyone running a local demo

This document describes how Thittam isolates tenants, how the UI adapts per
vertical, and how to stand up a new demo tenant alongside existing ones.

---

## 1. Isolation model

Thittam uses a **tenant-per-schema** isolation model. Each tenant's
business-domain tables live in a dedicated PostgreSQL schema named
`tenant_<uuid>`; cross-tenant reads are prevented at the connection level
rather than by query discipline. The tenant identifier flows end-to-end:

1. **Login** — `iam.Login` resolves the user's `tenant_id` and embeds it as a
   JWT claim.
2. **Gateway** — Kong / grpc-gateway forwards the JWT and copies the claim into
   an `X-Tenant-ID` header.
3. **Interceptor** — every vertical-aware gRPC handler asserts `X-Tenant-ID`
   matches the JWT claim; mismatches return `PERMISSION_DENIED`.
4. **Database connection** — `pkg/tenantdb.Acquire` sets
   `search_path = tenant_<uuid>` on the pooled connection before any query
   runs. Unqualified table references in sqlc-generated queries resolve to
   the current tenant's schema; reaching another tenant's rows is physically
   impossible on that connection. A `tenant_id` column on each row provides
   a secondary belt-and-braces check, not the primary isolation mechanism.
5. **Cache** — Redis keys are prefixed with `tenant:<uuid>:` so no data bleeds
   between tenants at the L2 cache layer either.

Shared/global tables (`tenants`, `users`, `tenant_verticals`, audit logs) live
in the `public` schema and are always queried with explicit `WHERE tenant_id`
filters — these are the identity-plane tables that *describe* tenants rather
than belonging to one.

Universal services (`iam`, `general-ledger`, `notifications`, `document`) do
not require a vertical binding. Vertical-aware services (`project`, `budget`,
`expense`, `inventory`, `reporting`) load the tenant's vertical config at
request time via an interceptor in `pkg/vertical/`.

## 2. Vertical plugin system

A **vertical** is a YAML file in `pkg/vertical/configs/` that defines:

- Entity labels (`project` / `phase` / `team_member`)
- Phase types and allowed transitions
- Budget categories with default account codes
- Expense types, inventory buckets, reporting templates
- Icons, colours, and default theme

A tenant binds to exactly one vertical via the `tenant_verticals` table. The
binding is captured at tenant provisioning time and drives every downstream
label the user sees.

### Currently shipped verticals

| Vertical ID            | Label                        | Status       |
| ---------------------- | ---------------------------- | ------------ |
| `movie-production`     | Movie & Media Production     | In demo use  |
| `construction`         | Construction & Civil Eng.    | Ready, not yet seeded |
| `software-development` | Software Development         | Ready        |
| `events-management`    | Events & Experiential        | Ready        |

See `pkg/vertical/configs/` for the source of truth.

## 3. Current demo tenants

| # | Tenant                            | Vertical           | Currency | Country | Seed directory                   | Status                            |
| - | --------------------------------- | ------------------ | -------- | ------- | -------------------------------- | --------------------------------- |
| 1 | XYZ_CBA Productions Pvt. Ltd.     | movie-production   | INR      | IN      | `seeds/demo/xyz-cba/`             | ✅ Seeded                         |
| 2 | XYZ Construction LLC              | construction       | USD      | US      | `seeds/demo/xyz-construction/`    | 🚧 Scaffold only; Phase A pending |

Both tenants coexist in the same dev database. Switching between them is a
logout-and-log-back-in operation; no configuration change is required.

## 4. Login matrix for local demos

All demo users share password `demo1234`.

### XYZ_CBA (movie-production)

| Role                  | Email                         |
| --------------------- | ----------------------------- |
| Super Admin / Owner   | rajesh.kumar@xyzcba.com       |
| Executive Producer    | priya.sharma@xyzcba.com       |
| Line Producer         | arun.nair@xyzcba.com          |
| Production Accountant | meena.iyer@xyzcba.com         |
| (…and 4 crew roles)   | see `seeds/demo/xyz-cba/002_users.sql` |

### XYZ Construction (construction) — once seeded

| Role                 | Email                                  |
| -------------------- | -------------------------------------- |
| Owner / Super Admin  | miles.sullivan@xyzconstruction.com     |
| Project Director     | dana.reyes@xyzconstruction.com         |
| Estimator            | ethan.choi@xyzconstruction.com         |
| Site Supervisor      | nora.patel@xyzconstruction.com         |
| Finance Controller   | raj.menon@xyzconstruction.com          |
| Procurement          | kim.alvarez@xyzconstruction.com        |

## 5. How the UI adapts per vertical

When a user authenticates, the `/me` endpoint returns the tenant's
`vertical_id` and `entity_labels`. The frontend reads these via
`useTheme()` / the auth context and substitutes them throughout:

| UI surface                          | Movie tenant         | Construction tenant  |
| ----------------------------------- | -------------------- | -------------------- |
| Sidebar nav item                    | "Productions"        | "Projects"           |
| Phase column label                  | "Phase"              | "Stage"              |
| Team member label                   | "Crew"               | "Site Staff"         |
| Budget detail category order        | ATL / BTL / Post     | Prelims → Materials → Labour → Subcontract → Plant → Contingency |
| Currency format                     | `₹12,34,567.89` (IN) | `$1,234,567.89` (US) |
| Default icons                       | Film-themed          | Hard-hat / crane     |

Label-aware components: `sidebar.tsx`, `topbar.tsx`, `page-header.tsx`,
budget/expense/inventory tables, and all filter dropdowns.

## 6. Adding a new tenant (operational runbook)

1. Confirm a suitable vertical YAML exists in `pkg/vertical/configs/`; author
   one if not. Run `make validate-verticals` to confirm it parses.
2. Pick a deterministic tenant UUID — demo tenants use the pattern
   `d0000000-0000-0000-0000-00000000000<N>`.
3. Create `seeds/demo/<slug>/` with numbered SQL files following the xyz-cba
   layout:
   - `001_tenant.sql` — `tenants` row + `tenant_verticals` binding
   - `002_users.sql` — demo users (all using the shared bcrypt hash)
   - `003_projects.sql` — `productions` rows (see Section 7 for the status
     CHECK constraint workaround)
   - `004_budgets.sql` — budget versions + line items
   - `005_expenses.sql` — actuals (optional; needed only if expense demos
     will be shown)
   - `006_inventory.sql`, `007_iam_roles.sql`, `008_ledger.sql`,
     `009_notification_templates.sql`, `010_document_folders.sql` — optional
4. Add a parallel Makefile target `seed-<slug>` so the new seed can be loaded
   independently without disturbing existing tenants.
5. Add an entry to Section 3 of this document and to the login matrix in
   Section 4.
6. Load with `make seed-<slug>` and verify by logging in as the owner user.

## 7. Known limitations

| # | Limitation                                                                                                                                          | Mitigation                                                                                       |
| - | --------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| 1 | `productions.status` has a CHECK constraint hardcoded to movie-production lifecycle values (`development`, `pre_production`, …, `archived`)         | Map vertical-specific stage IDs onto the existing allowed statuses in the seed (see xyz-construction plan §3). File an issue to replace with a tenant-aware validation. |
| 2 | Cross-tenant reporting is not supported — by design. Every query is scoped by `tenant_id`.                                                          | If ever needed, introduce a `platform` scope with strict RBAC and audit logging.                 |
| 3 | `SET search_path` is not parameterised — schema names must be interpolated into SQL. `pkg/tenantdb` is the only sanctioned path; ad-hoc interpolation elsewhere is a schema-injection vector. | Code review + `pkg/tenantdb.Acquire` enforces UUID-typed inputs (see the package doc comment).   |
| 4 | UI "tenant switcher" does not yet exist. Multi-tenant users (platform admins) must log out to switch.                                               | Planned post-v1.                                                                                 |

## 8. Related documents

- `docs/demo-xyz-construction-plan.md` — the specific roll-out plan for the
  construction demo tenant (Phase A/B/C).
- `pkg/vertical/README.md` (if present) — vertical plugin authoring guide.
- `seeds/demo/<slug>/README.md` — per-tenant fixture reference.
