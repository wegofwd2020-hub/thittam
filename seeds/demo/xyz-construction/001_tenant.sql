-- 001_tenant.sql — XYZ Construction LLC demo tenant
-- Vertical: construction | Plan: professional
-- Address: 123 Main Street, Milford, MI 48381, USA | Currency: USD

INSERT INTO tenants (
    id, name, slug, plan, status,
    address_line1, address_line2, city, country_code, postal_code,
    primary_currency_code
) VALUES (
    'd0000000-0000-0000-0000-000000000002',
    'XYZ Construction LLC',
    'xyz-construction',
    'professional',
    'active',
    '123 Main Street',
    NULL,
    'Milford',
    'US',
    '48381',
    'USD'
)
ON CONFLICT (id) DO NOTHING;

-- Bind tenant to construction vertical (registered_by = Miles Sullivan)
INSERT INTO tenant_verticals (tenant_id, vertical_id, registered_by) VALUES
    ('d0000000-0000-0000-0000-000000000002',
     'construction',
     'd1000000-0000-0000-0000-000000000201')
ON CONFLICT (tenant_id) DO NOTHING;
