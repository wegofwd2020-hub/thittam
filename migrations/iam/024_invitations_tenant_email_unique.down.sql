-- 024_invitations_tenant_email_unique.down.sql
ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_tenant_email_unique;
CREATE INDEX idx_invitations_email ON invitations (tenant_id, email);
