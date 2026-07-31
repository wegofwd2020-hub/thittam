-- 013_budget_line_items.sql — lines for the two budgets 004 left empty
--
-- 004_budgets.sql creates three budgets but gives line items to only the
-- first, so budget detail for Midnight Express Reboot and Project Starfall
-- rendered an empty table. That is an omission there rather than a failed
-- insert: all 13 of its line items reference d3...0001.
--
-- Two conventions are carried over from 004 rather than invented here:
--   * lines sum exactly to the budget's total_amount, so the detail page adds
--     up to the figure the budget itself claims;
--   * actual_amount and committed_amount are left at their column defaults of
--     zero. No expense in 005 is reconciled against these budgets, and seeding
--     spend that no transaction backs would make the demo assert a number the
--     rest of the data cannot support.
--
-- Categories and account codes match 004: above_the_line 5100,
-- below_the_line 5200, post_production 5300.
--
-- Deterministic UUIDs, prefix d4 continuing 004's series:
--   d4000000-0000-0000-0000-0000000004NN  budget d3...0002
--   d4000000-0000-0000-0000-0000000003NN  budget d3...0003
--
-- The 02NN block is NOT free: seed-construction loads a second vertical into
-- the same tables, and its five budgets already hold d4...0201 through
-- d4...0283. The d4 space is shared across verticals, not per tenant.

-- Budget d3...0002 — "V2 — Post VFX Overrun Revision", INR 12,00,00,000.
-- The label is the story: this is the revision where post ran over, so the
-- post_production block is disproportionately large against a shoot that has
-- already wrapped.
INSERT INTO budget_line_items
  (id, budget_id, tenant_id, category_id, description, account_code, budgeted_amount) VALUES
('d4000000-0000-0000-0000-000000000401', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'above_the_line',  'Director',                      '5100', '12000000.00'),
('d4000000-0000-0000-0000-000000000402', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'above_the_line',  'Lead Cast — ensemble',          '5100', '28000000.00'),
('d4000000-0000-0000-0000-000000000403', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'above_the_line',  'Screenwriter',                  '5100',  '2500000.00'),
('d4000000-0000-0000-0000-000000000404', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'below_the_line',  'Principal Photography Unit',    '5200', '22000000.00'),
('d4000000-0000-0000-0000-000000000405', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'below_the_line',  'Camera & Lighting Equipment',   '5200',  '6500000.00'),
('d4000000-0000-0000-0000-000000000406', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'below_the_line',  'Locations & Permits',           '5200',  '4000000.00'),
('d4000000-0000-0000-0000-000000000407', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'below_the_line',  'Art Department & Sets',         '5200',  '8000000.00'),
('d4000000-0000-0000-0000-000000000408', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'VFX — 340 shots',               '5300', '24000000.00'),
('d4000000-0000-0000-0000-000000000409', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'Editorial & Conform',           '5300',  '4500000.00'),
('d4000000-0000-0000-0000-000000000410', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'Sound Design & Final Mix',      '5300',  '5000000.00'),
('d4000000-0000-0000-0000-000000000411', 'd3000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'Music & Score',                 '5300',  '3500000.00')
ON CONFLICT (id) DO NOTHING;

-- Budget d3...0003 — "V1 — Initial Estimate", INR 4,00,00,000.
-- An animated feature still in development: seven coarse lines, no unit or
-- equipment costs, and the director unattached. 003_productions.sql records
-- the director as TBD, so a named line here would contradict it.
INSERT INTO budget_line_items
  (id, budget_id, tenant_id, category_id, description, account_code, budgeted_amount) VALUES
('d4000000-0000-0000-0000-000000000301', 'd3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'above_the_line',  'Director — allowance, unattached', '5100',  '5000000.00'),
('d4000000-0000-0000-0000-000000000302', 'd3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'above_the_line',  'Story & Screenplay',               '5100',  '3500000.00'),
('d4000000-0000-0000-0000-000000000303', 'd3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'below_the_line',  'Animation Production — outsourced', '5200', '18000000.00'),
('d4000000-0000-0000-0000-000000000304', 'd3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'below_the_line',  'Character & Environment Design',   '5200',  '4000000.00'),
('d4000000-0000-0000-0000-000000000305', 'd3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'below_the_line',  'Voice Cast',                       '5200',  '3000000.00'),
('d4000000-0000-0000-0000-000000000306', 'd3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'Editorial & Compositing',          '5300',  '3500000.00'),
('d4000000-0000-0000-0000-000000000307', 'd3000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'Sound & Music',                    '5300',  '3000000.00')
ON CONFLICT (id) DO NOTHING;
