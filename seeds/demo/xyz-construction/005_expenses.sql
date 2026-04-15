-- 005_expenses.sql — Actuals driving the "62% committed" / overrun stories
--
-- OPTIONAL for MVP. Only required if the demo plays through expense entry as
-- well as the budget plan view. If omitted, the `actual_amount` /
-- `committed_amount` values on budget_line_items (set in 004_budgets.sql) are
-- sufficient to paint the VarianceIndicator correctly.
--
-- Status: PLACEHOLDER — author in Phase A or defer to Phase A+.

-- TODO(seed): expense entries per project. Rough distribution to target:
--   Project 1  0 entries        (Tender — not yet spending)
--   Project 2  ~3 entries       (bonds, insurance, initial mobilisation)
--   Project 3  ~8 entries       (early construction — materials, subs)
--   Project 4  ~15 entries      (mid construction; includes MATERIALS OVERRUN
--                                line items that push that category red)
--   Project 5  ~25 entries      (near-complete; broad spread across categories)
--   Project 6  ~30 entries      (complete; final invoice reconciliation)
--
-- Expense schema reference: seeds/demo/xyz-cba/005_expenses.sql

SELECT 'xyz-construction 005_expenses.sql: placeholder — no rows inserted' AS status;
