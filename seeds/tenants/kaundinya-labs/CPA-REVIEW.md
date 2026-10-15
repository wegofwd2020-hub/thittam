# Kaundinya Labs LLC — chart of accounts and Form 1065 mapping (for CPA review)

**Status:** implemented in the seed generator (`openspec/changes/add-partnership-books`).
The CPA confirmed on 15 Oct 2026 that the company files **Form 1065**. The
opening entry is **not** prepared for posting: it waits on agenda Q1 and Q2.
**Entity:** two-member Michigan LLC, 50/50 (Member A / Member B), default classification → partnership, Form 1065 with Schedule K-1 per member.
**Books:** calendar fiscal year, USD. Opening date 2026-10-20.

Member names are not in this file. Members appear as Member A and Member B; in
the local books the account names carry the real names from the git-ignored
`company.local.yaml`.

Line numbers are for the **tax year 2025** Form 1065, page 1. On that form
"Other deductions" is **line 21** (line 20 is the energy-efficient buildings
deduction); an earlier draft of this packet said line 20. Recheck each year.

## 1. Equity accounts

Generated from the `entity:` block in `company.yaml`.

| Account | Name | Purpose |
|---|---|---|
| 3000 | Equity | parent (was "Owner's Capital"; renamed in place) |
| 3010 | Capital — Member A | contributions + allocated income/loss |
| 3020 | Capital — Member B | contributions + allocated income/loss |
| 3110 | Distributions — Member A | draws during the year; closed to 3010 at year end |
| 3120 | Distributions — Member B | draws during the year; closed to 3020 at year end |
| 3900 | Retained Earnings | kept from the vertical; normally unused in a partnership |
| 3910 | Current-Year Earnings (clearing) | allocated 50/50 to 3010 / 3020 at year-end close |

- **3100 Owner's Contributions** and **3200 Owner's Draws** are not created for
  a partnership. If the books were already seeded from the single-owner chart,
  re-seeding deactivates them (not deleted; history stays).

## 2. Assets for the opening entry

| Account | Name | Notes |
|---|---|---|
| 1300 | Intangible Assets | parent |
| 1310 | Software & Intellectual Property | Mentible as contributed; basis depends on Q2 |
| 1320 | Start-up & Organizational Costs | §195 / §709; current total $125.00 of LARA fees, build spend TBD |
| 1390 | Accumulated Amortization | contra-asset |
| 6860 | Amortization (expense) | 1065 line 21 via Form 4562 |

## 3. Guaranteed payments

| Account | Name | 1065 line |
|---|---|---|
| 6050 | Guaranteed Payments to Partners | 10 (parent) |
| 6051 | Guaranteed Payments — Member A | 10 |
| 6052 | Guaranteed Payments — Member B | 10 |

Guaranteed payments are for services or capital use, paid regardless of
income. The accounts exist so they are classified correctly if paid; leave
them at zero otherwise.

## 4. Chart of accounts → Form 1065 page 1

| Acct | Name | Form 1065 line | Note |
|---|---|---|---|
| 1000–1590 | Assets | balance sheet (Sch L) | |
| 2000 | Accounts Payable | balance sheet | |
| 2100 | Business Credit Card | balance sheet | |
| 2200 | Sales & Use Tax Payable | balance sheet | |
| 2300 | Due to Owners (reimbursable) | balance sheet | renamed from "Due to Owner" |
| 4000 / 4100 / 4200 | Revenue | 1a | |
| 4900 | Other Income | 7 | |
| 6050–6052 | Guaranteed payments | 10 | see §3 |
| 6100 | Contract Labor | 21 | statement; not wages (1099-NEC) |
| 6210 | Cloud & Hosting | 21 | statement — **CPA: label** |
| 6220 | Software & SaaS Subscriptions | 21 | statement — **CPA: label** |
| 6300 | Advertising & Marketing | 21 | |
| 6400 | Legal & Professional Fees | 21 | |
| 6500 | Travel | 21 | |
| 6510 | Business Meals (50% deductible) | 21 | 50% limit applied before this line |
| 6600 | Taxes & Licenses | 14 | |
| 6710 | Office Supplies & Expense | 21 | |
| 6720 | Business Use of Home | — | partners claim it themselves, not the firm — **CPA** |
| 6800 | Insurance | 21 | |
| 6820 | Bank & Payment Processing Fees | 21 | |
| 6850 | Depreciation | 16a | Form 4562 |
| 6860 | Amortization | 21 | Form 4562; §195/§709 costs |
| 6900 | Other Expenses | 21 | |

Line 9 (salaries and wages, other than to partners) stays unused while the
company has no W-2 employees. **CPA to confirm.**

## 5. Open questions for the CPA

1. **Q1 — liquidation / capital basis.** Opening capital per capital accounts or per percentage interest?
2. **Q2 — Mentible contribution.** Adjusted basis or fair market value, and which member contributed it?
3. **6210 / 6220** — statement labels on line 21.
4. **Business use of home (6720)** — unreimbursed partnership expenses on the partners' own returns, or reimbursed by the firm under an accountable plan?
5. **§195 start-up costs (1320)** — amortisation start date and period; which pre-formation build spend qualifies.
6. **Guaranteed payments (§3)** — will either member receive them?
7. **K-1 capital reporting** — Item L format (tax basis).
8. **Michigan** — partnership filing requirements for a two-member LLC.

## 6. Still to do

- Opening entry `2026-10-20`: draft once Q1 and Q2 are answered.
- K-1 allocation report (Item L roll-forward per member).
- Year-end close: allocate 3910 to 3010 / 3020 by percentage interest.
