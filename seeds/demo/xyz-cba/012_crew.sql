-- 012_crew.sql — XYZ_CBA crew: who is attached to each production
--
-- Crew size tracks the production's phase, because that is what makes the
-- three productions read as different rather than as the same list three
-- times: a shoot in progress carries a full unit, a picture in post carries a
-- handful of finishing roles, and a project in development carries almost
-- nobody.
--
-- user_id is deliberately NULL for external crew. 003_productions.sql already
-- describes production 1's director as external talent, and a freelance DoP or
-- gaffer has no login in the tenant. The column is nullable precisely so a crew
-- list is not restricted to people who happen to hold accounts — filling it in
-- with a staff id would misrepresent who these people are.
--
-- Day rates are INR (the column defaults to INR) and are ordinary industry
-- shapes: department heads well above unit crew, external specialists above
-- salaried staff.
--
-- Deterministic UUIDs, prefix d9.
--   d9000000-0000-0000-0000-0000000001NN  production 1
--   d9000000-0000-0000-0000-0000000002NN  production 2
--   d9000000-0000-0000-0000-0000000003NN  production 3

INSERT INTO crew_members
  (id, production_id, tenant_id, user_id, name, role, department, day_rate, start_date, end_date) VALUES

-- Production 1: "The Last Horizon" — full unit, mid-shoot.
('d9000000-0000-0000-0000-000000000101', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000002',
 'Priya Sharma',  'Executive Producer',     'Production', 25000, '2025-09-15', '2026-11-30'),
('d9000000-0000-0000-0000-000000000102', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000003',
 'Arun Nair',     'Line Producer',          'Production', 18000, '2026-01-05', '2026-08-23'),
('d9000000-0000-0000-0000-000000000103', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000004',
 'Meena Iyer',    'Production Manager',     'Production', 12000, '2026-01-05', '2026-08-23'),
('d9000000-0000-0000-0000-000000000104', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', NULL,
 'Kabir Raut',    'Director',               'Direction',  75000, '2025-10-01', '2026-11-30'),
('d9000000-0000-0000-0000-000000000105', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', NULL,
 'Nandita Rao',   'Director of Photography','Camera',     60000, '2026-06-30', '2026-08-23'),
('d9000000-0000-0000-0000-000000000106', 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001', NULL,
 'Suresh Pillai', 'Gaffer',                 'Lighting',    9000, '2026-06-30', '2026-08-23'),

-- Production 2: "Midnight Express Reboot" — finishing crew only.
('d9000000-0000-0000-0000-000000000201', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000005',
 'Vikram Reddy',  'Post Supervisor',        'Post-Production', 15000, '2026-01-19', '2026-09-30'),
('d9000000-0000-0000-0000-000000000202', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000007',
 'Karthik Rajan', 'VFX Producer',           'VFX',             20000, '2026-01-19', '2026-09-30'),
('d9000000-0000-0000-0000-000000000203', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', NULL,
 'Farah Qureshi', 'Editor',                 'Post-Production', 40000, '2025-11-10', '2026-06-30'),
('d9000000-0000-0000-0000-000000000204', 'd2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001', NULL,
 'Rohan Bhatt',   'Sound Designer',         'Sound',           35000, '2026-04-01', '2026-09-30'),

-- Production 3: "Project Starfall" — development only, three people.
('d9000000-0000-0000-0000-000000000301', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000001',
 'Rajesh Kumar',  'Executive Producer',     'Production',  25000, '2026-01-10', NULL),
('d9000000-0000-0000-0000-000000000302', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000008',
 'Ananya Das',    'Development Executive',  'Development', 10000, '2026-01-10', NULL),
('d9000000-0000-0000-0000-000000000303', 'd2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001', 'd1000000-0000-0000-0000-000000000006',
 'Deepa Menon',   'Storyboard Coordinator', 'Art',          8000, '2026-03-02', NULL)

ON CONFLICT (id) DO NOTHING;
