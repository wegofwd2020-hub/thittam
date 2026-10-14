# Kaundinya Labs LLC — chart of accounts and Form 1065 mapping (for CPA review)

**Status:** proposal. Not loaded into any database. Opening entry is **not** prepared for posting.
**Entity:** two-member Michigan LLC, 50/50 (Member A / Member B), default classification → partnership, Form 1065 with Schedule K-1 per member.
**Books:** calendar fiscal year, USD. Opening date 2026-10-20.

Member names are not in this file. Members appear as Member A and Member B; names are resolved from the git-ignored local override.

## 1. Equity accounts (proposed)

| Account | Name | Purpose | Replaces |
|---|---|---|---|
| 3010 | Capital — Member A | contributions + allocated income/loss | 3000 Owner's Capital (A's share) |
| 3020 | Capital — Member B | contributions + allocated income/loss | 3000 Owner's Capital (B's share) |
| 3110 | Distributions — Member A | draws during the year; closed to 3010 at year end | 3200 Owner's Draws (A) |
| 3120 | Distributions — Member B | draws during the year; closed to 3020 at year end | 3200 Owner's Draws (B) |
| 3910 | Current-year earnings (clearing) | allocated 50/50 to 3010 / 3020 at year-end close | — |

- **3900 is already used** in the current chart for "Retained Earnings", so the clearing account is 3910 instead of the number in the earlier handoff.
- **3100 Owner's Contributions** is deactivated in the migration (not deleted). Its history stays.
- The existing 3000 and 3200 accounts are deactivated in the same migration, so no history is lost.

## 2. Assets added for the opening entry (proposed)

| Account | Name | Notes |
|---|---|---|
| 1300 | Software / IP — Mentible | contributed asset; basis depends on CPA answer Q2 |
| 1310 | Deferred start-up costs (§195) | amortisation rules to confirm with CPA |

## 3. Guaranteed payments (proposed)

| Account | Name | 1065 line |
|---|---|---|
| 6050 | Guaranteed payments — Member A | 10 |
| 6055 | Guaranteed payments — Member B | 10 |

Guaranteed payments are for services or capital use, paid regardless of income. Only add these if the members actually receive them.

## 4. Chart of accounts → Form 1065 page 1 (proposed)

Line numbers are for Form 1065 page 1. **Verify against the current-year form before filing.**

| Acct | Name | Form 1065 line | Note |
|---|---|---|---|
| 1000 | Cash & Bank | balance sheet | |
| 1010 | Business Checking | balance sheet | opening deposit account |
| 1020 | Business Savings | balance sheet | |
| 1100 | Accounts Receivable | balance sheet | |
| 1200 | Prepaid Expenses | balance sheet | |
| 1500 | Equipment | balance sheet | |
| 1590 | Accumulated Depreciation | balance sheet | |
| 2000 | Accounts Payable | balance sheet | |
| 2100 | Business Credit Card | balance sheet | |
| 2200 | Sales & Use Tax Payable | balance sheet | |
| 2300 | Due to Owner (reimbursable) | balance sheet | rename to "Due to Member" — CPA to confirm |
| 4000 | Revenue | 1a | |
| 4100 | Product & Subscription Revenue | 1a | |
| 4200 | Services & Consulting Revenue | 1a | |
| 4900 | Other Income | 7 | |
| 6000 | Operating Expenses | 20 | parent; statement detail |
| 6100 | Contract Labor | 20 | not wages (1099-NEC); CPA to confirm |
| 6200 | Technology | 20 | statement |
| 6210 | Cloud & Hosting | 20 | statement — **CPA Q: 6210 vs 6220** |
| 6220 | Software & SaaS Subscriptions | 20 | statement — **CPA Q: 6210 vs 6220** |
| 6300 | Advertising & Marketing | 20 | no separate 1065 line |
| 6400 | Legal & Professional Fees | 20 | |
| 6500 | Travel | 20 | |
| 6510 | Business Meals (50% deductible) | 20 | 50% limit applied before this line |
| 6600 | Taxes & Licenses | 14 | |
| 6700 | Office | 20 | |
| 6710 | Office Supplies & Expense | 20 | |
| 6720 | Business Use of Home | 20 | **CPA Q:** partnership home-office treatment |
| 6800 | Insurance | 20 | |
| 6820 | Bank & Payment Processing Fees | 20 | |
| 6850 | Depreciation | 16 | |
| 6900 | Other Expenses | 20 | |
| 6050 / 6055 | Guaranteed payments | 10 | see §3 |

Line 9 (salaries and wages, other than to partners) is left unused. **CPA to confirm** that the company has no W-2 employees.

## 5. Questions for the CPA (15 Oct agenda)

1. **Q1 — liquidation / capital basis.** Opening capital per capital accounts or per percentage interest (the 50/50 split gift decision)?
2. **Q2 — Mentible contribution.** Adjusted basis or fair market value? Which member contributed it, and does the $500.00 cash contribution belong to that member?
3. **6210 vs 6220** for Schedule C-era cloud vs SaaS — which 1065 statement label?
4. **Business use of home (6720)** — is it allowed for a partnership, and what statement label?
5. **Contract labor (6100)** — statement "other deductions" or a separate line?
6. **§195 start-up costs (1310)** — amortisation start date and period.
7. **Guaranteed payments (§3)** — do either member receive them?
8. **K-1 capital reporting** — beginning and ending capital in Item L; confirm the format we report.
9. **Partnership state filing** — Michigan partnership return requirements for a two-member LLC.

## 6. Not yet done (follow-up after CPA answers)

- Config: `tax_classification` field and `members:` block in the tenant config, with validation and tests.
- Migration: deactivate the single-owner equity accounts, add the per-member accounts.
- Opening entry draft (`005_opening_entry.sql`): placeholders until Q1 and Q2 are answered.
- K-1 allocation report: stub with TODO until the structure is confirmed.
