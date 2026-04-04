-- 001_tenant.sql — XYZ_CBA Productions demo tenant
-- Vertical: movie-production | Plan: professional

INSERT INTO tenants (id, name, slug, plan, status) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'XYZ_CBA Productions Pvt. Ltd.', 'xyz-cba-productions', 'professional', 'active')
ON CONFLICT (id) DO NOTHING;

-- Bind tenant to movie-production vertical
INSERT INTO tenant_verticals (tenant_id, vertical_id, registered_by) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'movie-production', 'd1000000-0000-0000-0000-000000000001')
ON CONFLICT (tenant_id) DO NOTHING;
