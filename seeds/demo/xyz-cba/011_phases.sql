-- 011_phases.sql — XYZ_CBA production phases: the full lifecycle, three times over
--
-- Every production carries all five phase types from the project service's
-- vertical config (development, pre_production, production, post_production,
-- released). What differs is where each one has got to, so the demo shows the
-- same lifecycle caught at three different points rather than three variations
-- on "in progress".
--
-- Phase status is constrained to pending / active / completed. Exactly one
-- phase per production is active, and it matches that production's own status
-- in 003_productions.sql — a production sitting in 'post_production' whose
-- active phase said otherwise would be the demo contradicting itself.
--
-- Deterministic UUIDs, prefix d8 (d5 is expenses, d6/d7 inventory).
--   d8000000-0000-0000-0000-0000000001NN  production 1
--   d8000000-0000-0000-0000-0000000002NN  production 2
--   d8000000-0000-0000-0000-0000000003NN  production 3

INSERT INTO phases (id, production_id, tenant_id, phase_type, status, start_date, end_date) VALUES

-- Production 1: "The Last Horizon" — actively shooting, day 32 of 55.
-- Dates put the shoot around the present so the active phase reads as current
-- rather than as something that ended months ago.
('d8000000-0000-0000-0000-000000000101', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'development',     'completed', '2025-09-15', '2025-12-20'),
('d8000000-0000-0000-0000-000000000102', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'pre_production',  'completed', '2026-01-05', '2026-06-25'),
('d8000000-0000-0000-0000-000000000103', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'production',      'active',    '2026-06-30', '2026-08-23'),
('d8000000-0000-0000-0000-000000000104', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'pending',   '2026-08-24', '2026-11-30'),
('d8000000-0000-0000-0000-000000000105', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'released',        'pending',   NULL,         NULL),

-- Production 2: "Midnight Express Reboot" — picture lock done, VFX in progress.
('d8000000-0000-0000-0000-000000000201', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'development',     'completed', '2025-03-01', '2025-05-30'),
('d8000000-0000-0000-0000-000000000202', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'pre_production',  'completed', '2025-06-02', '2025-08-29'),
('d8000000-0000-0000-0000-000000000203', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'production',      'completed', '2025-09-08', '2026-01-16'),
('d8000000-0000-0000-0000-000000000204', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'active',    '2026-01-19', '2026-09-30'),
('d8000000-0000-0000-0000-000000000205', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'released',        'pending',   NULL,         NULL),

-- Production 3: "Project Starfall" — script V2 under review, storyboarding started.
-- Only the first phase has dates: an animated feature in development has no
-- credible shoot window yet, and inventing one would be the seed asserting
-- something the production itself does not know.
('d8000000-0000-0000-0000-000000000301', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'development',     'active',    '2026-01-10', '2026-10-30'),
('d8000000-0000-0000-0000-000000000302', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'pre_production',  'pending',   NULL,         NULL),
('d8000000-0000-0000-0000-000000000303', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'production',      'pending',   NULL,         NULL),
('d8000000-0000-0000-0000-000000000304', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'post_production', 'pending',   NULL,         NULL),
('d8000000-0000-0000-0000-000000000305', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'released',        'pending',   NULL,         NULL)

ON CONFLICT (id) DO NOTHING;
