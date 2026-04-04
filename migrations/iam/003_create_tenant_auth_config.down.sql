-- 003_create_tenant_auth_config.down.sql

DROP TRIGGER IF EXISTS trg_tenant_auth_config_updated_at ON tenant_auth_config;
DROP TABLE IF EXISTS tenant_auth_config;
