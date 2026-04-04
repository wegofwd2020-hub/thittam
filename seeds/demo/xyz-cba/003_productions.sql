-- 003_productions.sql — XYZ_CBA Productions: 3 productions in different phases
-- These would be in the tenant-scoped schema (tenant_d0000000...)
-- For demo purposes, we use a shared productions table.

-- Production 1: "The Last Horizon" — in Production phase (active shoot)
-- Genre: Sci-Fi Thriller | Director: External talent | Budget: ₹8.5 Cr
-- Status: Actively shooting, Day 32 of 55

-- Production 2: "Midnight Express Reboot" — in Post-Production
-- Genre: Action Drama | Director: In-house | Budget: ₹12 Cr
-- Status: Picture lock done, VFX in progress, sound mix pending

-- Production 3: "Project Starfall" — in Development phase
-- Genre: Animated Feature | Director: TBD | Budget: ₹4 Cr (estimated)
-- Status: Script V2 under review, storyboarding started

-- Note: Actual table creation depends on the project-management service schema.
-- This file documents the demo data structure for seeding once the service exists.

-- Deterministic UUIDs for cross-referencing
-- Production 1: d2000000-0000-0000-0000-000000000001
-- Production 2: d2000000-0000-0000-0000-000000000002
-- Production 3: d2000000-0000-0000-0000-000000000003

/*
INSERT INTO productions (id, tenant_id, title, description, phase, created_by, created_at) VALUES

('d2000000-0000-0000-0000-000000000001',
 'd0000000-0000-0000-0000-000000000001',
 'The Last Horizon',
 'A sci-fi thriller set in 2087 where a research team discovers a signal from beyond the observable universe. Filming across Mumbai, Goa, and Ladakh.',
 'production',
 'd1000000-0000-0000-0000-000000000002',  -- Priya Sharma (Exec Producer)
 '2025-09-15T10:00:00Z'),

('d2000000-0000-0000-0000-000000000002',
 'd0000000-0000-0000-0000-000000000001',
 'Midnight Express Reboot',
 'A modern reimagining of the classic action drama. Set in contemporary Mumbai underworld. Featuring an ensemble cast.',
 'post_production',
 'd1000000-0000-0000-0000-000000000002',
 '2025-03-01T10:00:00Z'),

('d2000000-0000-0000-0000-000000000003',
 'd0000000-0000-0000-0000-000000000001',
 'Project Starfall',
 'An animated feature film targeting family audiences. A young girl discovers she can communicate with stars. India''s first large-scale animated theatrical release by XYZ_CBA.',
 'development',
 'd1000000-0000-0000-0000-000000000001',  -- Rajesh Kumar (Owner)
 '2026-01-10T10:00:00Z')

ON CONFLICT (id) DO NOTHING;
*/
