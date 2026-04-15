-- 002_users.sql — XYZ Construction LLC demo users
-- All users share password "demo1234" (same bcrypt hash as xyz-cba):
--   $2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku
--
-- Status: PLACEHOLDER — to be authored in Phase A.

-- TODO(seed): author 6 user rows mirroring seeds/demo/xyz-cba/002_users.sql.
--
-- Users share the cross-tenant d1000000-... namespace documented in
-- xyz-cba's README. This tenant uses the -2XX offset to remain distinct
-- from xyz-cba's -00X range:
--
--   d1000000-0000-0000-0000-000000000201  Miles Sullivan     Owner / Super Admin
--   d1000000-0000-0000-0000-000000000202  Dana Reyes         Project Director
--   d1000000-0000-0000-0000-000000000203  Ethan Choi         Estimator
--   d1000000-0000-0000-0000-000000000204  Nora Patel         Site Supervisor
--   d1000000-0000-0000-0000-000000000205  Raj Menon          Finance Controller
--   d1000000-0000-0000-0000-000000000206  Kim Alvarez        Procurement
--
-- INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES
--     ('d1000000-0000-0000-0000-000000000201',
--      'd0000000-0000-0000-0000-000000000002',
--      'miles.sullivan@xyzconstruction.com',
--      'Miles Sullivan',
--      '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
--      'active'),
--     -- … 5 more …
-- ON CONFLICT (id) DO NOTHING;

SELECT 'xyz-construction 002_users.sql: placeholder — no rows inserted' AS status;
