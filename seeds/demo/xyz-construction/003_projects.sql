-- 003_projects.sql — XYZ Construction LLC projects (in `productions` table)
-- 6 projects at varying construction stages.
--
-- `productions.status` has a CHECK constraint hardcoded to the movie-production
-- lifecycle. Construction stages are mapped onto those values until the
-- constraint is relaxed (see docs/multi-tenancy.md §7):
--
--   Tender         → development
--   Mobilisation   → pre_production
--   Construction   → production
--   Commissioning  → post_production
--   Handover       → released
--   Completed      → archived

INSERT INTO productions (
    id, tenant_id, title, slug, description, status,
    start_date, end_date, created_by, created_at
) VALUES

-- 1. Oakwood Medical Plaza — Tender (draft budget)
('d2000000-0000-0000-0000-000000000201',
 'd0000000-0000-0000-0000-000000000002',
 'Oakwood Medical Plaza',
 'oakwood-medical-plaza',
 '3-storey medical office building, 48,000 sqft. Ann Arbor, MI. Tender phase — bid package under preparation.',
 'development',
 '2026-06-01', '2027-10-30',
 'd1000000-0000-0000-0000-000000000203',  -- Ethan Choi (Estimator)
 '2026-03-10T09:00:00Z'),

-- 2. Riverbend Logistics Hub — Mobilisation (budget submitted)
('d2000000-0000-0000-0000-000000000202',
 'd0000000-0000-0000-0000-000000000002',
 'Riverbend Logistics Hub',
 'riverbend-logistics-hub',
 '250,000 sqft tilt-up logistics warehouse with 40 dock doors. Toledo, OH. Mobilisation underway; budget awaiting Director approval.',
 'pre_production',
 '2026-02-15', '2027-08-31',
 'd1000000-0000-0000-0000-000000000203',  -- Ethan Choi
 '2026-01-20T09:00:00Z'),

-- 3. Cedar Park Townhomes (Phase I) — Early Construction (approved, healthy)
('d2000000-0000-0000-0000-000000000203',
 'd0000000-0000-0000-0000-000000000002',
 'Cedar Park Townhomes (Phase I)',
 'cedar-park-townhomes-phase-i',
 '24-unit townhome development, wood-frame over podium. Novi, MI. Framing in progress; ~25% committed.',
 'production',
 '2025-11-01', '2026-12-20',
 'd1000000-0000-0000-0000-000000000203',
 '2025-10-05T09:00:00Z'),

-- 4. Great Lakes Brewery Expansion — Mid Construction (approved, MATERIALS OVERRUN)
('d2000000-0000-0000-0000-000000000204',
 'd0000000-0000-0000-0000-000000000002',
 'Great Lakes Brewery Expansion',
 'great-lakes-brewery-expansion',
 'Production floor expansion + tank farm, process-piping heavy. Grand Rapids, MI. Materials line 8% over budget; Contingency partial draw in progress.',
 'production',
 '2025-09-01', '2026-09-30',
 'd1000000-0000-0000-0000-000000000203',
 '2025-08-10T09:00:00Z'),

-- 5. Huron Valley Water Treatment Retrofit — Commissioning (locked, ~91%)
('d2000000-0000-0000-0000-000000000205',
 'd0000000-0000-0000-0000-000000000002',
 'Huron Valley Water Treatment Retrofit',
 'huron-valley-water-treatment-retrofit',
 'Filtration and UV upgrade at municipal WTP. Milford, MI. Construction complete; equipment commissioning + punchlist.',
 'post_production',
 '2025-04-01', '2026-05-31',
 'd1000000-0000-0000-0000-000000000203',
 '2025-03-05T09:00:00Z'),

-- 6. Midtown Office Renovation — Handover (locked, complete)
('d2000000-0000-0000-0000-000000000206',
 'd0000000-0000-0000-0000-000000000002',
 'Midtown Office Renovation',
 'midtown-office-renovation',
 '18,000 sqft Class-A office fit-out, 4th and 5th floors. Detroit, MI. Substantial completion reached; final reconciliation.',
 'released',
 '2025-02-01', '2026-02-28',
 'd1000000-0000-0000-0000-000000000203',
 '2025-01-12T09:00:00Z')

ON CONFLICT (id) DO NOTHING;
