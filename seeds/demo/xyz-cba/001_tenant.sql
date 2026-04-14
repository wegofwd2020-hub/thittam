-- 001_tenant.sql — XYZ_CBA Productions demo tenant
-- Vertical: movie-production | Plan: professional

INSERT INTO tenants (
    id, name, slug, plan, status,
    address_line1, address_line2, city, country_code, postal_code, primary_currency_code
) VALUES (
    'd0000000-0000-0000-0000-000000000001',
    'XYZ_CBA Productions Pvt. Ltd.',
    'xyz-cba-productions',
    'professional',
    'active',
    'Plot 42, Film City Road',
    'Goregaon East',
    'Mumbai',
    'IN',
    '400065',
    'INR'
)
ON CONFLICT (id) DO NOTHING;

-- Bind tenant to movie-production vertical
INSERT INTO tenant_verticals (tenant_id, vertical_id, registered_by) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'movie-production', 'd1000000-0000-0000-0000-000000000001')
ON CONFLICT (tenant_id) DO NOTHING;
