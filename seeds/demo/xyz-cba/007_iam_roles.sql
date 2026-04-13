-- 007_iam_roles.sql — System roles and user role assignments for XYZ_CBA Productions
-- Mirrors the systemRoles seeded by iam.Service.seedSystemRoles at tenant creation.
-- Run after 002_users.sql.
--
-- Role UUID pattern: e0000000-0000-0000-0000-00000000000X
-- Tenant:           d0000000-0000-0000-0000-000000000001

-- ═══════════════════════════════════════════════════════════
-- System Roles (ADR-014 generic names; vertical labels via role_labels in YAML)
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
 'manager',
 ARRAY['production:read','production:write','budget:read','budget:approve',
       'expense:approve','inventory:checkout','report:read'],
 true),

('e0000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001',
 'coordinator',
 ARRAY['production:read','production:write','budget:read','budget:write',
       'expense:approve','inventory:checkout','report:read'],
 true),

('e0000000-0000-0000-0000-000000000004',
 'd0000000-0000-0000-0000-000000000001',
 'accountant',
 ARRAY['budget:read','expense:submit','expense:approve','report:read'],
 true),

('e0000000-0000-0000-0000-000000000006',
 'd0000000-0000-0000-0000-000000000001',
 'member',
 ARRAY['production:read','expense:submit'],
 true),

('e0000000-0000-0000-0000-000000000007',
 'd0000000-0000-0000-0000-000000000001',
 'inventory_manager',
 ARRAY['inventory:read','inventory:write','inventory:checkout','inventory:retire'],
 true),

-- project_supervisor permissions are tenant-wide in Phase 1.
-- Phase 2 (#42) adds project_id scoping — Vikram Reddy will then be bound to
-- production d2000000-...-001 (The Last Horizon) instead of tenant-wide.
('e0000000-0000-0000-0000-000000000008',
 'd0000000-0000-0000-0000-000000000001',
 'project_supervisor',
 ARRAY['production:read','budget:read','expense:submit','expense:approve',
       'resource:manage','inventory:checkout'],
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

-- Priya Sharma → manager (was: executive_producer)
('d1000000-0000-0000-0000-000000000002',
 'e0000000-0000-0000-0000-000000000002',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Arun Nair → coordinator (was: line_producer)
('d1000000-0000-0000-0000-000000000003',
 'e0000000-0000-0000-0000-000000000003',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Meena Iyer → accountant (was: production_accountant)
('d1000000-0000-0000-0000-000000000004',
 'e0000000-0000-0000-0000-000000000004',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Vikram Reddy → project_supervisor (was: department_head; ADR-014 reassignment)
-- Intent: scoped to The Last Horizon (d2000000-...-001). Tenant-wide today;
-- project_id constraint enforced in Phase 2.
('d1000000-0000-0000-0000-000000000005',
 'e0000000-0000-0000-0000-000000000008',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Deepa Menon → member (was: crew_member)
('d1000000-0000-0000-0000-000000000006',
 'e0000000-0000-0000-0000-000000000006',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Karthik Rajan → member (was: crew_member)
('d1000000-0000-0000-0000-000000000007',
 'e0000000-0000-0000-0000-000000000006',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z'),

-- Ananya Das → member (was: crew_member)
('d1000000-0000-0000-0000-000000000008',
 'e0000000-0000-0000-0000-000000000006',
 'd1000000-0000-0000-0000-000000000001',
 '2025-09-01T09:00:00Z')

ON CONFLICT (user_id, role_id) DO NOTHING;
