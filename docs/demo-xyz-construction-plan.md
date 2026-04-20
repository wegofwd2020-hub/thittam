# Demo Plan — XYZ Construction

**Status:** Draft, awaiting start of Phase A
**Author:** Initial plan captured 2026-04-15
**Goal:** Stand up a second demo tenant (construction vertical) with six
projects at varying stages so the `/budgets` UI can be demonstrated end-to-end
across every budget-status state.

---

## 1. Tenant profile

| Field               | Value                                       |
| ------------------- | ------------------------------------------- |
| Tenant name         | `XYZ Construction LLC`                      |
| Slug                | `xyz-construction`                          |
| Plan                | `professional`                              |
| Status              | `active`                                    |
| Address             | 123 Main Street, Milford, MI 48381, USA     |
| Country code        | `US`                                        |
| Primary currency    | `USD` (country-driven per #61)              |
| Vertical binding    | `construction` (YAML already exists)        |
| Tenant UUID         | `d0000000-0000-0000-0000-000000000002`      |

Seed directory: `seeds/demo/xyz-construction/` — mirrors the
`seeds/demo/xyz-cba/` layout.

## 2. Users (6)

All share password `demo1234` (bcrypt cost 12, same hash as xyz-cba).

| Name             | Role                  | Email                                 | Demo purpose              |
| ---------------- | --------------------- | ------------------------------------- | ------------------------- |
| Miles Sullivan   | Owner / Super Admin   | miles.sullivan@xyzconstruction.com    | Login demo                |
| Dana Reyes       | Project Director      | dana.reyes@xyzconstruction.com        | Approves budgets          |
| Ethan Choi       | Estimator             | ethan.choi@xyzconstruction.com        | Creates draft budgets     |
| Nora Patel       | Site Supervisor       | nora.patel@xyzconstruction.com        | Records actuals           |
| Raj Menon        | Finance Controller    | raj.menon@xyzconstruction.com         | Locks approved budgets    |
| Kim Alvarez      | Procurement           | kim.alvarez@xyzconstruction.com       | Subcontract line items    |

## 3. Six projects at varying stages

| # | Project                                    | Location         | Value   | Stage                | Budget Status | Spend Story                                              |
| - | ------------------------------------------ | ---------------- | ------- | -------------------- | ------------- | -------------------------------------------------------- |
| 1 | Oakwood Medical Plaza                      | Ann Arbor, MI    | $4.8M   | Tender               | Draft         | Pre-award estimate; no actuals                           |
| 2 | Riverbend Logistics Hub                    | Toledo, OH       | $12.5M  | Mobilisation         | Submitted     | Awaiting Director approval; 2% spent on bonds/insurance  |
| 3 | Cedar Park Townhomes (Phase I)             | Novi, MI         | $8.2M   | Construction (early) | Approved      | 25% committed; on-budget; healthy                        |
| 4 | Great Lakes Brewery Expansion              | Grand Rapids, MI | $3.1M   | Construction (mid)   | Approved      | 62% committed; **Materials over 8%** — contingency draw  |
| 5 | Huron Valley Water Treatment Retrofit      | Milford, MI      | $6.7M   | Commissioning        | Locked        | 91% spent; minor punchlist; showcases locked-budget UI   |
| 6 | Midtown Office Renovation                  | Detroit, MI      | $2.4M   | Handover             | Locked        | 100% spent; final reconciliation                         |

### Stage → status mapping (workaround for Q2)

The `productions` table has a CHECK constraint hardcoded to movie-production
statuses. For the demo seed we map construction stages onto the existing
allowed values so no schema change is required:

| Construction stage | `productions.status`  |
| ------------------ | --------------------- |
| Tender             | `development`         |
| Mobilisation       | `pre_production`      |
| Construction       | `production`          |
| Commissioning      | `post_production`     |
| Handover           | `released`            |
| Completed          | `archived`            |

**Follow-up (post-demo):** file an issue to drop the CHECK constraint so
vertical YAMLs become the source of truth for allowed project statuses.

## 4. Budget plan shape per project

Each project has **one budget version** with line items across the six
construction categories from `pkg/vertical/configs/construction.yaml`:

| Category                 | % of total (typical) | Example line items                                |
| ------------------------ | -------------------- | ------------------------------------------------- |
| Preliminaries            | 6–8%                 | Site setup, temp power, fencing, mobilisation     |
| Materials & Procurement  | 30–40%               | Concrete, rebar, framing, MEP rough-in materials  |
| Direct Labour            | 15–20%               | Foremen, carpenters, masons, safety officers      |
| Subcontract Works        | 20–25%               | MEP, drywall, roofing, sitework subs              |
| Plant & Equipment        | 5–8%                 | Crane, excavator, scaffolding, lifts              |
| Contingency              | 5–10%                | Unallocated reserve                               |

### Per-project budget differentiation

| # | What the budget demonstrates                                                            |
| - | --------------------------------------------------------------------------------------- |
| 1 | Empty-state / draft budget — round-number top-line estimate, few or no line items       |
| 2 | Full BOQ breakdown (~15 items), awaiting approval — "Submitted" badge                    |
| 3 | Healthy in-progress: small `actual_amount` + `committed_amount` values (all green)       |
| 4 | **Money shot** — Materials `actual` > `budgeted`; contingency partial draw (red rows)    |
| 5 | Locked budget, ~91% consumed; mostly green totals; line items non-editable               |
| 6 | Locked + complete; actual ≈ budgeted; change-order reconciliation                        |

## 5. Implementation phases

### Phase A — Seed data (backend, ~1 day)

1. `seeds/demo/xyz-construction/001_tenant.sql`
   — tenant row + `tenant_verticals` binding to `construction`
2. `002_users.sql` — 6 users with bcrypt(`demo1234`)
3. `003_projects.sql` — 6 rows in `productions` table (status via stage map above)
4. `004_budgets.sql` — 6 `budgets` rows + ~70–100 `budget_line_items`
5. `005_expenses.sql` *(optional for MVP)* — actuals driving Project 4's overrun story
6. Generalise `make seed` target to accept `TENANT=xyz-construction`

### Phase B — UI data swap (frontend, ~½–1 day) — **blocks the demo**

7. `web/src/app/(dashboard)/budgets/page.tsx` — replace `mockBudgets` with
   `useQuery({ queryKey: ['budgets'], queryFn: listBudgets })` from
   `@/lib/api/budgets`
8. `web/src/app/(dashboard)/budgets/[id]/page.tsx` — replace
   `mockBudgets[params.id]` with `getBudget(id)` + `listLineItems(id)`
9. Add loading + empty states
10. Update `budgets-journey.spec.ts` to use real project names

### Phase C — Demo polish (~½ day)

11. Walk through as Miles Sullivan: projects 1 → 6 in order
12. Playwright smoke per budget state (draft, submitted, approved,
    approved+overrun, locked, locked+complete)

## 6. Investigation findings (Q1–Q4 resolved)

| # | Question                           | Finding                                                                                                    |
| - | ---------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| 1 | USD formatting in `AmountDisplay`  | ✅ Works out of the box — `$4,800,000.00` format via `en-US` locale                                         |
| 2 | `productions` table vs. vertical   | ⚠️ Table name is universal; status CHECK is movie-specific — use stage→status map in §3                     |
| 3 | Sidebar vertical labels            | ✅ Already reads `entityLabels.projectPlural` via `useTheme()` — shows "Projects" for construction          |
| 4 | Overrun / variance UI              | ✅ `VarianceIndicator` paints green / amber (≥80%) / red (over) per line item and totals                   |

## 7. Timeline

**2–3 working days** end-to-end, assuming no regressions uncovered during
Phase B when the budgets page is moved from mock to real data.
