# XYZ_CBA Productions — Demo Seed Data

## Company Profile

| Field | Value |
|---|---|
| Company | XYZ_CBA Productions Pvt. Ltd. |
| Vertical | movie-production |
| Plan | professional |
| Tenant ID | `d0000000-0000-0000-0000-000000000001` |

## Demo Users

All users share password: `demo1234`

| Name | Email | Role | ID |
|---|---|---|---|
| Rajesh Kumar | rajesh.kumar@xyzcba.com | Super Admin / Owner | `d1..001` |
| Priya Sharma | priya.sharma@xyzcba.com | Executive Producer | `d1..002` |
| Arun Nair | arun.nair@xyzcba.com | Line Producer | `d1..003` |
| Meena Iyer | meena.iyer@xyzcba.com | Production Accountant | `d1..004` |
| Vikram Reddy | vikram.reddy@xyzcba.com | Production Manager | `d1..005` |
| Deepa Menon | deepa.menon@xyzcba.com | Crew (Cinematographer) | `d1..006` |
| Karthik Rajan | karthik.rajan@xyzcba.com | Crew (Art Director) | `d1..007` |
| Ananya Das | ananya.das@xyzcba.com | Crew (Sound Designer) | `d1..008` |

## Productions

| Title | Phase | Budget | Notes |
|---|---|---|---|
| **The Last Horizon** | Production (Day 32/55) | ₹8.5 Cr (approved) | Sci-fi thriller; shooting Mumbai/Goa/Ladakh |
| **Midnight Express Reboot** | Post-Production | ₹12 Cr (draft V2) | Action drama; VFX in progress |
| **Project Starfall** | Development | ₹4 Cr (draft V1) | Animated feature; script review |

## What to Look For in Demos

### Budget Management
- "The Last Horizon" has an approved V1 budget with 13 line items across ATL/BTL/Post categories
- "Midnight Express Reboot" V2 shows a revision triggered by VFX cost overrun
- "Project Starfall" V1 is an early estimate — useful for showing draft workflow

### Expense Tracking
- 20 expenses covering all statuses: paid, approved, submitted, rejected, draft
- Expense #14 (₹2,00,000) is at the exact `coordinator` approval limit
- Expense #15 (₹35,00,000) was rejected for exceeding the BTL budget line
- 4 petty cash entries (₹2,200 to ₹15,000)
- Vendor GSTIN populated on PO-required expenses

### Inventory
- 3 cameras (2 checked out to "The Last Horizon", 1 available)
- Props: sci-fi panels, costumes, breakaway furniture
- Drone and gimbal available (returned from previous production)

### Approval Workflow
- Line Producer (Arun Nair) can approve up to ₹2,00,000
- Executive Producer (Priya Sharma) can approve up to ₹10,00,000
- Dual approval required above ₹10,00,000

## Seed Files

| File | Contents | Depends On |
|---|---|---|
| `001_tenant.sql` | Tenant record + vertical binding | shared migrations |
| `002_users.sql` | 8 demo users (all roles, password: `demo1234`) | 001 |
| `003_productions.sql` | 3 productions in different phases | 002 |
| `004_budgets.sql` | Budget versions + 13 ATL/BTL/Post line items | 003 |
| `005_expenses.sql` | 20 expenses (paid, approved, submitted, rejected, draft) | 003 |
| `006_inventory.sql` | 8 equipment/prop assets | 001 |
| `007_iam_roles.sql` | 6 system roles + user role assignments | 002 |
| `008_ledger.sql` | Chart of accounts (16 accounts) + 4 open accounting periods | 001 |
| `009_notification_templates.sql` | 9 templates for expense/budget/document events | 001 |
| `010_document_folders.sql` | 13 folders (tenant-wide + production-scoped) | 003 |

## Loading the Seed Data

```bash
# Prerequisites: all migrations must be run first
make migrate-all

# Load all seed files in order (each is idempotent — safe to re-run)
DB="postgres://thittam:thittam_dev@localhost:5432/thittam?sslmode=disable"
for f in seeds/demo/xyz-cba/*.sql; do
  echo "Loading $f ..."
  psql $DB -f "$f"
done
```

For detailed testing instructions see:
[`thittam_docs/docs/operations/local-testing-guide.md`](https://github.com/wegofwd2020-hub/thittam_docs/blob/main/docs/operations/local-testing-guide.md)

## UUID Convention

All demo UUIDs follow a predictable pattern for easy cross-referencing:

| Entity | Pattern | Example |
|---|---|---|
| Tenant | `d0000000-...-00X` | `d0000000-0000-0000-0000-000000000001` |
| Users | `d1000000-...-00X` | `d1000000-0000-0000-0000-000000000001` |
| Productions | `d2000000-...-00X` | `d2000000-0000-0000-0000-000000000001` |
| Budgets | `d3000000-...-00X` | `d3000000-0000-0000-0000-000000000001` |
| Line Items | `d4000000-...-0XX` | `d4000000-0000-0000-0000-000000000001` |
| Expenses | `d5000000-...-0XX` | `d5000000-0000-0000-0000-000000000001` |
| Assets | `d6000000-...-00X` | `d6000000-0000-0000-0000-000000000001` |
