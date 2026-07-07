-- Grant thittam_app least privilege and enforce audit_log append-only.
-- Run as the DB OWNER (thittam) AFTER all migrations (GRANT ON ALL TABLES needs
-- every table to exist). Idempotent. Does NOT create the role — CREATE ROLE needs
-- superuser (see local-db-init.sh / the CI provisioning step).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'thittam_app') THEN
        RAISE EXCEPTION 'role thittam_app does not exist — create it first (local-db-init.sh or CI provisioning)';
    END IF;
END $$;

GRANT USAGE ON SCHEMA public TO thittam_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO thittam_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO thittam_app;

-- Future tables/sequences created by thittam auto-grant to thittam_app.
-- NB: a FUTURE audit table would also get UPDATE/DELETE here and would need its
-- own REVOKE (only audit_log is revoked below).
ALTER DEFAULT PRIVILEGES FOR ROLE thittam IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO thittam_app;
ALTER DEFAULT PRIVILEGES FOR ROLE thittam IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO thittam_app;

-- Append-only enforcement: audit_log is INSERT/SELECT only for the app role.
REVOKE UPDATE, DELETE ON audit_log FROM thittam_app;
