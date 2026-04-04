-- 004_create_platform_users.down.sql

DROP INDEX IF EXISTS idx_impersonation_tenant;
DROP INDEX IF EXISTS idx_impersonation_platform_user;
DROP TABLE IF EXISTS platform_impersonation_log;
DROP INDEX IF EXISTS idx_platform_users_status;
DROP INDEX IF EXISTS idx_platform_users_email;
DROP TABLE IF EXISTS platform_users;
