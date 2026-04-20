-- 002_users.sql — XYZ Construction LLC demo users
-- Password for all demo accounts: "demo1234" (same bcrypt hash as xyz-cba)
-- bcrypt("demo1234", cost=12) = $2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku
--
-- Users share the cross-tenant d1000000-... namespace. Second-tenant
-- entities use the -2XX offset (see seeds/demo/xyz-cba/README.md).

INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES

-- Miles Sullivan — Owner / Super Admin
('d1000000-0000-0000-0000-000000000201',
 'd0000000-0000-0000-0000-000000000002',
 'miles.sullivan@xyzconstruction.com',
 'Miles Sullivan',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Dana Reyes — Project Director
('d1000000-0000-0000-0000-000000000202',
 'd0000000-0000-0000-0000-000000000002',
 'dana.reyes@xyzconstruction.com',
 'Dana Reyes',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Ethan Choi — Estimator
('d1000000-0000-0000-0000-000000000203',
 'd0000000-0000-0000-0000-000000000002',
 'ethan.choi@xyzconstruction.com',
 'Ethan Choi',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Nora Patel — Site Supervisor
('d1000000-0000-0000-0000-000000000204',
 'd0000000-0000-0000-0000-000000000002',
 'nora.patel@xyzconstruction.com',
 'Nora Patel',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Raj Menon — Finance Controller
('d1000000-0000-0000-0000-000000000205',
 'd0000000-0000-0000-0000-000000000002',
 'raj.menon@xyzconstruction.com',
 'Raj Menon',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Kim Alvarez — Procurement
('d1000000-0000-0000-0000-000000000206',
 'd0000000-0000-0000-0000-000000000002',
 'kim.alvarez@xyzconstruction.com',
 'Kim Alvarez',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active')

ON CONFLICT (tenant_id, email) DO NOTHING;
