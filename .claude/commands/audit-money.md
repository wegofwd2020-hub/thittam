---
description: Find violations of Rule #1 (money is never a float) across the Go codebase
---

Audit the codebase for Rule #1 violations.

## What counts as a violation

- `float32` or `float64` used for anything that represents money: amounts, rates, balances, fees, prices, currencies, totals, subtotals, taxes, discounts, conversions.
- Proto fields of type `double` or `float` near a monetary concept (the proto must use `string`; see `/new-proto`).
- SQL columns like `DOUBLE PRECISION`, `REAL`, `FLOAT` in `*.sql` migration files for monetary data. Correct type is `NUMERIC(14,2)`.
- JSON fields emitting numeric literals for money (should be strings with 2 decimal places).

## What does NOT count

- `float64` for **percentages**, **ratios**, **coordinates**, **probabilities**, **durations in seconds**, **benchmark timings**, **physical measurements**. Only flag when the context is money.
- Test fixtures constructing `decimal.Decimal` via `decimal.NewFromFloat(...)` — the stored type is still decimal.

## How to search

Run these Greps and combine results:

1. `float64` near monetary keywords across `.go` files
2. `float32` near monetary keywords across `.go` files
3. `double` or `float` in `.proto` files
4. `DOUBLE PRECISION`, `REAL`, `FLOAT` in `*.sql` migration files

Keywords to match near the float type (case-insensitive, within 3 lines):
`amount|price|cost|fee|total|subtotal|balance|currency|money|tax|discount|revenue|budget|expense|salary|wage`

## Output

A single markdown table, **zero false positives**. If you're unsure whether a match is money-related, read the surrounding 10 lines of context before deciding.

| File:line | Field / variable | Snippet | Suggested fix |
|---|---|---|---|
| services/billing/models.go:87 | `InvoiceTotal float64` | `InvoiceTotal float64 \`json:"total"\`` | Change to `decimal.Decimal` |

End with: `N violations found` or `No violations — Rule #1 is clean`.
