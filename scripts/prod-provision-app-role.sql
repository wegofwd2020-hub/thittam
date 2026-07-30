-- prod-provision-app-role.sql — create (or reset the password of) the prod
-- thittam_app runtime role. Idempotent. Run as a SUPERUSER against the prod DB.
-- The password is supplied as a psql variable so it never lands in git:
--
--   psql "$PROD_SUPERUSER_DSN" -v ON_ERROR_STOP=1 \
--        -v app_password="$(<read from your secret manager>)" \
--        -f scripts/prod-provision-app-role.sql
--
-- Then, as the OWNER (thittam), apply least privilege + the audit_log REVOKE:
--   make db-grant-app-role DB_URL="$PROD_OWNER_DSN"
-- (scripts/db-grant-app-role.sql — unchanged, not duplicated here.)

\if :{?app_password}
\else
  \warn '=> ERROR: pass the password with  -v app_password=...  (see header). Aborting.'
  \quit
\endif

-- Create the role if absent, otherwise just (re)set its password. Both branches
-- leave a LOGIN role named thittam_app with the supplied password and nothing
-- else — least-privilege GRANTs are applied separately by db-grant-app-role.sql.
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'thittam_app') AS need_create \gset
\if :need_create
  CREATE ROLE thittam_app LOGIN PASSWORD :'app_password';
\else
  ALTER ROLE thittam_app LOGIN PASSWORD :'app_password';
\endif
