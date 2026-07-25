-- 024_invitations_tenant_email_unique.up.sql
-- CreateInvitation upserts on (tenant_id, email) so re-inviting an address
-- refreshes its token; that ON CONFLICT target requires a UNIQUE constraint,
-- which the original table lacked (only token was UNIQUE). Replace the
-- non-unique lookup index with the unique constraint — its backing index
-- serves the same (tenant_id, email) lookups.
DROP INDEX IF EXISTS idx_invitations_email;
ALTER TABLE invitations
    ADD CONSTRAINT invitations_tenant_email_unique UNIQUE (tenant_id, email);
