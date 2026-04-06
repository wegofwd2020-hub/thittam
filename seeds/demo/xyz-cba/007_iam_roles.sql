-- 007_iam_roles.sql — System roles and user role assignments for XYZ_CBA Productions
-- Mirrors the systemRoles seeded by iam.Service.seedSystemRoles at tenant creation.
-- Run after 002_users.sql.
--
-- Role UUID pattern: e0000000-0000-0000-0000-00000000000X
-- Tenant:           d0000000-0000-0000-0000-000000000001

-- ═══════════════════════════════════════════════════════════
-- System Roles
-- ═══════════════════════════════════════════════════════════

INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES

('e0000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'super_admin',
 ARRAY['production:read','production:write','budget:read','budget:write','budget:approve',
       'expense:submit','expense:approve','inventory:checkout','report:read','user:manage'],
 true),

('e0000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001',
 'executive_producer',
 ARRAY['production:read','production:write','budget:read','budget:approve',
       'expense:approve','inventory:checkout','report:read'],
 true),

('e0000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001',
 'line_producer',
 ARRAY['production:read','production:write','budget:read','budget:write',
       'expense:approve','inventory:checkout','report:read'],
 true),

('e0000000-0000-0000-0000-000000000004',
 'd0000000-0000-0000-0000-000000000001',
 'production_accountant',
 ARRAY['budget:read','expense:submit','expense:approve','report:read'],
 true),

('e0000000-0000-0000-0000-000000000005',
 'd0000000-0000-0000-0000-000000000001',
 'department_head',
 ARRAY['production:read','expense:submit','inventory:checkout'],
 true),

('e0000000-0000-0000-0000-000000000006',
 'd0000000-0000-0000-0000-000000000001',
 'crew_member',
 ARRAY['production:read','expense:submit'],
 true)

ON CONFLICT (tenant_id, name) DO NOTHING;

-- ═══════════════════════════════════════════════════════════
-- User Role Assignments
-- assigned_by = Rajesh Kumar (super_admin / owner)
-- ═══════════════════════════════════════════════════════════

INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at) VALUES

-- Rajesh Kumar → super_admin
('d1000000-0000-0000-0000-000000000001',
 'e0000000-0000-0000-0000-000000000001',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Priya Sharma → executive_producer
('d1000000-0000-0000-0000-000000000002',
 'e0000000-0000-0000-0000-000000000002',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Arun Nair → line_producer
('d1000000-0000-0000-0000-000000000003',
 'e0000000-0000-0000-0000-000000000003',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Meena Iyer → production_accountant
('d1000000-0000-0000-0000-000000000004',
 'e0000000-0000-0000-0000-000000000004',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Vikram Reddy → department_head
('d1000000-0000-0000-0000-000000000005',
 'e0000000-0000-0000-0000-000000000005',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Deepa Menon → crew_member
('d1000000-0000-0000-0000-000000000006',
 'e0000000-0000-0000-0000-000000000006',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Karthik Rajan → crew_member
('d1000000-0000-0000-0000-000000000007',
 'e0000000-0000-0000-0000-000000000006',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Ananya Das → crew_member
('d1000000-0000-0000-0000-000000000008',
 'e0000000-0000-0000-0000-000000000006',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z')

ON CONFLICT (user_id, role_id) DO NOTHING;
