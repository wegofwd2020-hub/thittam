# XYZ Construction LLC — Demo Seed Data

**Status:** ✅ Phase A authored — seed loads 1 tenant, 6 users, 6 projects,
6 budgets, 83 line items, 28 expenses.
**Plan document:** `docs/demo-xyz-construction-plan.md`

## Company Profile

| Field               | Value                                       |
| ------------------- | ------------------------------------------- |
| Company             | XYZ Construction LLC                        |
| Vertical            | construction                                |
| Plan                | professional                                |
| Tenant ID           | `d0000000-0000-0000-0000-000000000002`      |
| Registered address  | 123 Main Street, Milford, MI 48381, USA     |
| Primary currency    | USD (country-driven via `US` country code)  |

## Demo Users

All users share password: `demo1234` (same bcrypt hash as xyz-cba).

| Name              | Email                                    | Role                   | ID         |
| ----------------- | ---------------------------------------- | ---------------------- | ---------- |
| Miles Sullivan    | miles.sullivan@xyzconstruction.com       | Owner / Super Admin    | `d1..201`  |
| Dana Reyes        | dana.reyes@xyzconstruction.com           | Project Director       | `d1..202`  |
| Ethan Choi        | ethan.choi@xyzconstruction.com           | Estimator              | `d1..203`  |
| Nora Patel        | nora.patel@xyzconstruction.com           | Site Supervisor        | `d1..204`  |
| Raj Menon         | raj.menon@xyzconstruction.com            | Finance Controller     | `d1..205`  |
| Kim Alvarez       | kim.alvarez@xyzconstruction.com          | Procurement            | `d1..206`  |

**UUID convention.** This tenant reuses the cross-tenant scheme documented
in `seeds/demo/xyz-cba/README.md` — users in the `d1000000-...` namespace,
projects in `d2000000-...`, budgets in `d3000000-...`, line items in
`d4000000-...`, expenses in `d5000000-...`. Second-tenant entities use the
`...-2XX` offset (`...-201` onwards) to stay distinct from xyz-cba's
`...-00X` range. Tenants live in separate schemas, so there is no
database-level collision either way; the offset is purely for human
readability when grepping across seed files.

## Projects

Six projects at varying stages, chosen to walk a demo reviewer through every
budget-status state the UI supports:

| # | Project                                 | Location         | Value   | Stage                | Budget Status | Demo story                          |
| - | --------------------------------------- | ---------------- | ------- | -------------------- | ------------- | ----------------------------------- |
| 1 | Oakwood Medical Plaza                   | Ann Arbor, MI    | $4.8M   | Tender               | Draft         | Pre-award; no line items            |
| 2 | Riverbend Logistics Hub                 | Toledo, OH       | $12.5M  | Mobilisation         | Submitted     | Awaiting Director approval           |
| 3 | Cedar Park Townhomes (Phase I)          | Novi, MI         | $8.2M   | Construction (early) | Approved      | 25% committed; healthy              |
| 4 | Great Lakes Brewery Expansion           | Grand Rapids, MI | $3.1M   | Construction (mid)   | Approved      | **Materials 8% over** — money shot  |
| 5 | Huron Valley Water Treatment Retrofit   | Milford, MI      | $6.7M   | Commissioning        | Locked        | 91% spent; punchlist                |
| 6 | Midtown Office Renovation               | Detroit, MI      | $2.4M   | Handover             | Locked        | 100% spent; final reconciliation    |

## Stage → `productions.status` mapping

The `productions` table's status CHECK constraint predates the vertical plugin
system and only accepts movie-production lifecycle values. Until that is
relaxed (see `docs/multi-tenancy.md` §7), construction stages are mapped onto
the existing allowed statuses for seed purposes:

| Construction stage | `productions.status` used |
| ------------------ | ------------------------- |
| Tender             | `development`             |
| Mobilisation       | `pre_production`          |
| Construction       | `production`              |
| Commissioning      | `post_production`         |
| Handover           | `released`                |
| Completed          | `archived`                |

## Budget Categories (from `construction.yaml`)

| Category                 | Typical %   | Example line items                                    |
| ------------------------ | ----------- | ----------------------------------------------------- |
| Preliminaries            | 6–8%        | Site setup, temp power, fencing, mobilisation         |
| Materials & Procurement  | 30–40%      | Concrete, rebar, framing, MEP rough-in materials      |
| Direct Labour            | 15–20%      | Foremen, carpenters, masons, safety officers          |
| Subcontract Works        | 20–25%      | MEP, drywall, roofing, sitework subs                  |
| Plant & Equipment        | 5–8%        | Crane, excavator, scaffolding, lifts                  |
| Contingency              | 5–10%       | Unallocated reserve                                   |

## What to Look For in Demos

### Budget Management
- **Project 1** — shows the empty-state / draft workflow
- **Project 2** — shows the submitted-for-approval state with reviewer actions
- **Project 3** — healthy green-across-the-board in-progress budget
- **Project 4** — **the money shot**: Materials line over budget, triggering
  red `VarianceIndicator` rows and a Contingency partial draw
- **Project 5** — locked budget UI; line items non-editable
- **Project 6** — locked + complete; change-order reconciliation visible

### Vertical-Aware UI
- Sidebar shows "Projects" instead of "Productions"
- Phase column labelled "Stage"
- Currency renders as `$1,234,567.89`
- Icons pick up construction-flavoured variants (hard hat, crane)

## Loading the seed

```bash
make seed-construction     # loads seeds/demo/xyz-construction/*.sql in order
```

The existing `make seed` target is unchanged and loads only xyz-cba. The two
tenants coexist in the same dev DB — see `docs/multi-tenancy.md` §3.

## File index

| File                      | Purpose                                        | Status           |
| ------------------------- | ---------------------------------------------- | ---------------- |
| `001_tenant.sql`          | Tenant row + vertical binding                  | ✅ Authored      |
| `002_users.sql`           | 6 demo users with bcrypt(`demo1234`)           | ✅ Authored      |
| `003_projects.sql`        | 6 rows in `productions` table                  | ✅ Authored      |
| `004_budgets.sql`         | 6 budget versions + 83 line items              | ✅ Authored      |
| `005_expenses.sql`        | 28 expenses, concentrated on Project 4 overrun | ✅ Authored      |
