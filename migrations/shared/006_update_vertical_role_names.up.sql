-- 006_update_vertical_role_names.up.sql
-- ADR-014 Phase 1 follow-up (#45): the embedded JSON in vertical_definitions
-- still references the old IAM role names (executive_producer, line_producer,
-- production_accountant, department_head, crew_member) inside
-- approval_workflow.limits[].role. After migration 011 renamed those roles in
-- iam.roles, LimitForRole(callerRole) was returning nil for renamed callers.
--
-- This migration rewrites the embedded JSON for existing rows. The seed file
-- (003_seed_vertical_definitions.up.sql) was also fixed in place so fresh
-- installs apply the correct names without depending on this migration.
--
-- Idempotent — jsonb_set on an already-renamed value is a no-op; the
-- regex-based replacement uses literal old strings that no longer exist after
-- a successful run.

-- Text-replace via the JSON serialisation. Simpler than walking the nested
-- jsonb paths since several role keys can appear in approval_workflow.limits[]
-- and the column has no other field that contains these literal strings.
UPDATE vertical_definitions
   SET config = (
       replace(
         replace(
           replace(
             replace(
               replace(config::text,
                 '"executive_producer"', '"manager"'),
               '"line_producer"', '"coordinator"'),
             '"production_accountant"', '"accountant"'),
           '"department_head"', '"accountant"'),
         '"crew_member"', '"member"'
       )
   )::jsonb
 WHERE config::text LIKE ANY (ARRAY[
     '%"executive_producer"%',
     '%"line_producer"%',
     '%"production_accountant"%',
     '%"department_head"%',
     '%"crew_member"%'
 ]);
