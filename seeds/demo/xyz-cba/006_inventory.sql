-- 006_inventory.sql — Inventory items for XYZ_CBA Productions
-- Depends on inventory-management service schema.

/*
INSERT INTO assets (id, tenant_id, category_id, name, description, serial_number, is_available, checked_out_to, depreciation_years) VALUES

-- Camera equipment (checked out to "The Last Horizon")
('d6000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001',
 'camera', 'ARRI Alexa Mini LF #1', 'Primary A-camera with PL mount', 'ARRI-K1234567',
 false, 'd2000000-0000-0000-0000-000000000001', 5),

('d6000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000001',
 'camera', 'ARRI Alexa Mini LF #2', 'B-camera for multi-angle setups', 'ARRI-K7654321',
 false, 'd2000000-0000-0000-0000-000000000001', 5),

('d6000000-0000-0000-0000-000000000003', 'd0000000-0000-0000-0000-000000000001',
 'camera', 'Sony FX6', 'BTS and documentary unit', 'SONY-FX6-98765',
 true, NULL, 5),

-- Props (not trackable — consumable/disposable)
('d6000000-0000-0000-0000-000000000004', 'd0000000-0000-0000-0000-000000000001',
 'props', 'Sci-fi control panels (set of 8)', 'Custom fabricated hero props for bridge set', NULL,
 false, 'd2000000-0000-0000-0000-000000000001', NULL),

('d6000000-0000-0000-0000-000000000005', 'd0000000-0000-0000-0000-000000000001',
 'props', 'Period costumes — lead cast', '12 costume sets for principal actors', NULL,
 false, 'd2000000-0000-0000-0000-000000000001', NULL),

('d6000000-0000-0000-0000-000000000006', 'd0000000-0000-0000-0000-000000000001',
 'props', 'Breakaway furniture set', 'Stunt-safe chairs and tables for fight sequence', NULL,
 true, NULL, NULL),

-- Available camera equipment (returned from Midnight Express)
('d6000000-0000-0000-0000-000000000007', 'd0000000-0000-0000-0000-000000000001',
 'camera', 'DJI Inspire 3 Drone', 'Aerial cinematography drone with Zenmuse X9', 'DJI-INS3-44556',
 true, NULL, 5),

('d6000000-0000-0000-0000-000000000008', 'd0000000-0000-0000-0000-000000000001',
 'camera', 'Movi Pro Gimbal', 'Freefly Movi Pro stabiliser', 'MOVI-PRO-33221',
 true, NULL, 5)

ON CONFLICT (id) DO NOTHING;
*/
