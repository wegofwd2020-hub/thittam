-- 006_inventory.sql — Inventory assets for XYZ_CBA Productions
-- Depends on inventory-management service schema.
-- Checkout records (which production an asset is assigned to) go in asset_checkouts.

-- Asset UUID pattern: d6000000-0000-0000-0000-00000000000X

INSERT INTO assets (id, tenant_id, asset_code, name, category_id, description, serial_number, status) VALUES

-- Camera equipment
('d6000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001',
 'CAM-001', 'ARRI Alexa Mini LF #1', 'camera',
 'Primary A-camera with PL mount', 'ARRI-K1234567', 'checked_out'),

('d6000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000001',
 'CAM-002', 'ARRI Alexa Mini LF #2', 'camera',
 'B-camera for multi-angle setups', 'ARRI-K7654321', 'checked_out'),

('d6000000-0000-0000-0000-000000000003', 'd0000000-0000-0000-0000-000000000001',
 'CAM-003', 'Sony FX6', 'camera',
 'BTS and documentary unit', 'SONY-FX6-98765', 'available'),

-- Props
('d6000000-0000-0000-0000-000000000004', 'd0000000-0000-0000-0000-000000000001',
 'PROP-001', 'Sci-fi control panels (set of 8)', 'props',
 'Custom fabricated hero props for bridge set', NULL, 'checked_out'),

('d6000000-0000-0000-0000-000000000005', 'd0000000-0000-0000-0000-000000000001',
 'PROP-002', 'Period costumes — lead cast', 'props',
 '12 costume sets for principal actors', NULL, 'checked_out'),

('d6000000-0000-0000-0000-000000000006', 'd0000000-0000-0000-0000-000000000001',
 'PROP-003', 'Breakaway furniture set', 'props',
 'Stunt-safe chairs and tables for fight sequence', NULL, 'available'),

-- Available equipment (returned from Midnight Express)
('d6000000-0000-0000-0000-000000000007', 'd0000000-0000-0000-0000-000000000001',
 'CAM-004', 'DJI Inspire 3 Drone', 'camera',
 'Aerial cinematography drone with Zenmuse X9', 'DJI-INS3-44556', 'available'),

('d6000000-0000-0000-0000-000000000008', 'd0000000-0000-0000-0000-000000000001',
 'CAM-005', 'Movi Pro Gimbal', 'camera',
 'Freefly Movi Pro stabiliser', 'MOVI-PRO-33221', 'available')

ON CONFLICT (id) DO NOTHING;

-- Checkout records for assets currently checked out to "The Last Horizon"
INSERT INTO asset_checkouts (id, asset_id, production_id, tenant_id, checked_out_to, checked_out_at, expected_return) VALUES

('d7000000-0000-0000-0000-000000000001',
 'd6000000-0000-0000-0000-000000000001',
 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'd1000000-0000-0000-0000-000000000006',  -- Deepa Menon (Cinematographer)
 '2025-10-15T08:00:00Z', '2026-05-01'),

('d7000000-0000-0000-0000-000000000002',
 'd6000000-0000-0000-0000-000000000002',
 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'd1000000-0000-0000-0000-000000000006',
 '2025-10-15T08:00:00Z', '2026-05-01'),

('d7000000-0000-0000-0000-000000000003',
 'd6000000-0000-0000-0000-000000000004',
 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'd1000000-0000-0000-0000-000000000007',  -- Karthik Rajan (Art Director)
 '2025-10-20T08:00:00Z', '2026-05-01'),

('d7000000-0000-0000-0000-000000000004',
 'd6000000-0000-0000-0000-000000000005',
 'd2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'd1000000-0000-0000-0000-000000000007',
 '2025-10-20T08:00:00Z', '2026-05-01')

ON CONFLICT (id) DO NOTHING;
