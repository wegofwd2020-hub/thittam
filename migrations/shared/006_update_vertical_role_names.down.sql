-- 006_update_vertical_role_names.down.sql
-- Restores the pre-ADR-014 movie-production role names. Lossy by design:
-- the up migration collapses department_head into accountant, so the down
-- migration cannot recover the distinction. Tenants that ran the up
-- migration and then rolled back will have department_head limits that
-- previously read from accountant.

UPDATE vertical_definitions
   SET config = (
       replace(
         replace(
           replace(
             replace(config::text,
               '"manager"', '"executive_producer"'),
             '"coordinator"', '"line_producer"'),
           '"accountant"', '"production_accountant"'),
         '"member"', '"crew_member"'
       )
   )::jsonb
 WHERE config::text LIKE ANY (ARRAY[
     '%"manager"%',
     '%"coordinator"%',
     '%"accountant"%',
     '%"member"%'
 ]);
