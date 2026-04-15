-- 002_users.sql — XYZ Construction LLC demo users
-- All users share password "demo1234" (same bcrypt hash as xyz-cba):
--   $2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku
--
-- Status: PLACEHOLDER — to be authored in Phase A.

-- TODO(seed): author 6 user rows mirroring seeds/demo/xyz-cba/002_users.sql.
--
-- User ID prefix: d2000000-0000-0000-0000-000000000XXX
--
-- User roster (see README.md for full detail):
--   001  Miles Sullivan     miles.sullivan@xyzconstruction.com     Owner / Super Admin
--   002  Dana Reyes         dana.reyes@xyzconstruction.com         Project Director
--   003  Ethan Choi         ethan.choi@xyzconstruction.com         Estimator
--   004  Nora Patel         nora.patel@xyzconstruction.com         Site Supervisor
--   005  Raj Menon          raj.menon@xyzconstruction.com          Finance Controller
--   006  Kim Alvarez        kim.alvarez@xyzconstruction.com        Procurement
--
-- INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES
--     ('d2000000-0000-0000-0000-000000000001',
--      'd0000000-0000-0000-0000-000000000002',
--      'miles.sullivan@xyzconstruction.com',
--      'Miles Sullivan',
--      '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
--      'active'),
--     -- … 5 more …
-- ON CONFLICT (id) DO NOTHING;

SELECT 'xyz-construction 002_users.sql: placeholder — no rows inserted' AS status;
