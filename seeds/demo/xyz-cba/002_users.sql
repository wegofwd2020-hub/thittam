-- 002_users.sql — XYZ_CBA Productions demo users
-- Password for all demo accounts: "demo1234" (bcrypt hash below)
-- bcrypt("demo1234", cost=12) = $2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku

INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES

-- Rajesh Kumar — Super Admin / Owner
('d1000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'rajesh.kumar@xyzcba.com',
 'Rajesh Kumar',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Priya Sharma — Executive Producer
('d1000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001',
 'priya.sharma@xyzcba.com',
 'Priya Sharma',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Arun Nair — Line Producer
('d1000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001',
 'arun.nair@xyzcba.com',
 'Arun Nair',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Meena Iyer — Production Accountant
('d1000000-0000-0000-0000-000000000004',
 'd0000000-0000-0000-0000-000000000001',
 'meena.iyer@xyzcba.com',
 'Meena Iyer',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Vikram Reddy — Production Manager
('d1000000-0000-0000-0000-000000000005',
 'd0000000-0000-0000-0000-000000000001',
 'vikram.reddy@xyzcba.com',
 'Vikram Reddy',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Deepa Menon — Crew Member (Cinematographer)
('d1000000-0000-0000-0000-000000000006',
 'd0000000-0000-0000-0000-000000000001',
 'deepa.menon@xyzcba.com',
 'Deepa Menon',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Karthik Rajan — Crew Member (Art Director)
('d1000000-0000-0000-0000-000000000007',
 'd0000000-0000-0000-0000-000000000001',
 'karthik.rajan@xyzcba.com',
 'Karthik Rajan',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active'),

-- Ananya Das — Crew Member (Sound Designer)
('d1000000-0000-0000-0000-000000000008',
 'd0000000-0000-0000-0000-000000000001',
 'ananya.das@xyzcba.com',
 'Ananya Das',
 '$2a$12$uCCJ3cY.t.dfAa6wiCJNYeGjqEQFpqHLc1iiVOQmsjaBYzZW1yeku',
 'active')

ON CONFLICT (tenant_id, email) DO NOTHING;
