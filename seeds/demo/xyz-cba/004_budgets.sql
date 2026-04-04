-- 004_budgets.sql — Budget versions for XYZ_CBA Productions
-- Depends on project-management and budget-planning service schemas.

-- Budget UUIDs:
-- d3000000-0000-0000-0000-000000000001  "The Last Horizon" V1 (Approved)
-- d3000000-0000-0000-0000-000000000002  "Midnight Express Reboot" V2 (Draft — revision)
-- d3000000-0000-0000-0000-000000000003  "Project Starfall" V1 (Draft)

/*
-- ═══════════════════════════════════════════════════════════
-- "The Last Horizon" — Approved Budget V1 (₹8.5 Cr)
-- ═══════════════════════════════════════════════════════════
INSERT INTO budget_versions (id, production_id, tenant_id, label, status, currency, total_amount, approved_by, approved_at, created_by) VALUES
('d3000000-0000-0000-0000-000000000001',
 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'V1 — Original Budget',
 'approved',
 'INR',
 '85000000.00',
 'd1000000-0000-0000-0000-000000000002',  -- Priya Sharma
 '2025-10-01T14:00:00Z',
 'd1000000-0000-0000-0000-000000000003');  -- Arun Nair

-- Line items (Above the Line)
INSERT INTO budget_line_items (id, budget_id, tenant_id, category_id, description, account_code, budgeted_amount) VALUES
('d4000000-0000-0000-0000-000000000001', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'above_the_line', 'Director — Sanjay Verma', '5100', '15000000.00'),
('d4000000-0000-0000-0000-000000000002', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'above_the_line', 'Lead Actor — Aarav Kapoor', '5100', '20000000.00'),
('d4000000-0000-0000-0000-000000000003', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'above_the_line', 'Lead Actress — Nisha Reddy', '5100', '12000000.00'),
('d4000000-0000-0000-0000-000000000004', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'above_the_line', 'Screenwriter', '5100', '3000000.00');

-- Line items (Below the Line)
INSERT INTO budget_line_items (id, budget_id, tenant_id, category_id, description, account_code, budgeted_amount) VALUES
('d4000000-0000-0000-0000-000000000005', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'below_the_line', 'Cinematographer — Deepa Menon', '5200', '5000000.00'),
('d4000000-0000-0000-0000-000000000006', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'below_the_line', 'Art Director — Karthik Rajan', '5200', '3500000.00'),
('d4000000-0000-0000-0000-000000000007', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'below_the_line', 'Camera & Lighting Equipment', '5200', '4500000.00'),
('d4000000-0000-0000-0000-000000000008', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'below_the_line', 'Location Rentals (Mumbai, Goa, Ladakh)', '5200', '6000000.00'),
('d4000000-0000-0000-0000-000000000009', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'below_the_line', 'Crew (55 shoot days × 40 crew)', '5200', '8000000.00'),
('d4000000-0000-0000-0000-000000000010', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'below_the_line', 'Transport & Logistics', '5200', '2000000.00');

-- Line items (Post-Production)
INSERT INTO budget_line_items (id, budget_id, tenant_id, category_id, description, account_code, budgeted_amount) VALUES
('d4000000-0000-0000-0000-000000000011', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'post_production', 'VFX (120 shots)', '5300', '4000000.00'),
('d4000000-0000-0000-0000-000000000012', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'post_production', 'Sound Design — Ananya Das', '5300', '1500000.00'),
('d4000000-0000-0000-0000-000000000013', 'd3000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 'post_production', 'Music & Score', '5300', '500000.00');

-- ═══════════════════════════════════════════════════════════
-- "Midnight Express Reboot" — Draft Budget V2 (₹12 Cr, revision)
-- ═══════════════════════════════════════════════════════════
INSERT INTO budget_versions (id, production_id, tenant_id, label, status, currency, total_amount, created_by) VALUES
('d3000000-0000-0000-0000-000000000002',
 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001',
 'V2 — Post VFX Overrun Revision',
 'draft',
 'INR',
 '120000000.00',
 'd1000000-0000-0000-0000-000000000003');

-- ═══════════════════════════════════════════════════════════
-- "Project Starfall" — Draft Budget V1 (₹4 Cr, estimated)
-- ═══════════════════════════════════════════════════════════
INSERT INTO budget_versions (id, production_id, tenant_id, label, status, currency, total_amount, created_by) VALUES
('d3000000-0000-0000-0000-000000000003',
 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001',
 'V1 — Initial Estimate',
 'draft',
 'INR',
 '40000000.00',
 'd1000000-0000-0000-0000-000000000001');
*/
