# Design: least-privilege `thittam_app` role — audit_log append-only (#120)

**Epic:** #120 (SQL-foundation subtask) · **Children:** #122 (DSN split), #123 (ops secret) — deferred.
**Date:** 2026-07-07
**Scope:** DB role bootstrap SQL + Makefile/script wiring + CI provisioning + a proof integration test.
**Branch:** `feat/db-app-role-audit-120`

## Context

The platform uses **one Postgres role, `thittam`, everywhere**, and it *owns* every table
(`scripts/local-db-init.sh`: `CREATE DATABASE thittam OWNER thittam`). A table owner
bypasses `REVOKE` in Postgres, so the audit migration's commented-out
`REVOKE UPDATE, DELETE ON audit_log FROM thittam_app` cannot enforce anything today. The
audit Store (#92, merged) is the first prod writer to `audit_log`, so this now matters.

This lands the **enforcement primitive**: a distinct, non-owner `thittam_app` role with
least privilege, with `UPDATE`/`DELETE` revoked on `audit_log`, plus an integration test
that *proves* the denial by connecting as `thittam_app`. Rewiring services to actually
connect as `thittam_app` (#122) and provisioning the prod secret (#123) are deferred.

### Hard constraint discovered

`CREATE ROLE` requires superuser or `CREATEROLE`. Local `thittam` is a plain `LOGIN`
role (`rolcreaterole = f`), so **role creation must happen in a privileged context**:
- Local: `scripts/local-db-init.sh` already runs as `sudo -u postgres` (superuser).
- CI: the Postgres service is provisioned with `POSTGRES_USER: thittam`, making CI's
  `thittam` a **superuser** — so a CI step can create `thittam_app`.

`GRANT`/`REVOKE`, by contrast, run fine as the **owner** `thittam` (owner grants on its
own tables). So the work splits: **role creation = privileged step; grants/revoke =
owner step (post-migrate).**

**Verification reality:** this sandbox's `thittam` cannot create roles and `sudo` hangs,
so the proof test **cannot run locally** — it runs in **CI** (which provisions
`thittam_app`). The test skips gracefully when its role DSN is absent.

## Component 1 — role creation (privileged)

`scripts/local-db-init.sh` — after the `thittam` role/db creation, add an idempotent
`thittam_app` creation (same `sudo -u postgres` pattern, password `DB_APP_PASS`
default `thittam_app_dev`):

```bash
DB_APP_USER=${DB_APP_USER:-thittam_app}
DB_APP_PASS=${DB_APP_PASS:-thittam_app_dev}
# ... SELECT 1 FROM pg_roles WHERE rolname=DB_APP_USER → skip, else:
#     CREATE ROLE thittam_app LOGIN PASSWORD '...';
```

`thittam_app` is created with **no** `CREATEDB`/`CREATEROLE`/`SUPERUSER` and is **not**
made the owner of anything — it only receives explicit grants (Component 2).

CI provisioning is Component 4.

## Component 2 — grant/revoke SQL (owner step)

New `scripts/db-grant-app-role.sql` — run as owner `thittam` **after** `migrate-all`
(all tables must exist for `GRANT ON ALL TABLES`). Idempotent (`GRANT`/`REVOKE` are).
Does NOT create the role; guards with a clear error if it's missing:

```sql
-- Grant thittam_app least privilege and enforce audit_log append-only.
-- Run as the DB owner (thittam) AFTER all migrations. Role must already exist
-- (created by local-db-init.sh / CI provisioning — CREATE ROLE needs superuser).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'thittam_app') THEN
        RAISE EXCEPTION 'role thittam_app does not exist — create it first (local-db-init.sh or CI)';
    END IF;
END $$;

GRANT USAGE ON SCHEMA public TO thittam_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO thittam_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO thittam_app;

-- Future tables/sequences created by thittam auto-grant to thittam_app.
ALTER DEFAULT PRIVILEGES FOR ROLE thittam IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO thittam_app;
ALTER DEFAULT PRIVILEGES FOR ROLE thittam IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO thittam_app;

-- Append-only enforcement: audit_log is INSERT/SELECT only for the app role.
REVOKE UPDATE, DELETE ON audit_log FROM thittam_app;
```

Note: `ALTER DEFAULT PRIVILEGES` covers future tables but would also grant UPDATE/DELETE
on any *future* audit table — only `audit_log` needs the revoke today; a follow-up audit
table would need its own revoke (documented in the SQL comment).

## Component 3 — Makefile + bootstrap wiring

`Makefile` — new target:

```make
db-grant-app-role:
	psql "$(DB_URL)" -v ON_ERROR_STOP=1 -f scripts/db-grant-app-role.sql
```

Call it after `migrate-all` in `scripts/db-bootstrap.sh` and
`scripts/db-test-bootstrap.sh` (against `TEST_DB_URL`). Because these local scripts use
the `sudo` init path (which hangs in this sandbox), local invocation is unchanged in
spirit — CI drives migrate + grant directly (Component 4).

## Component 4 — CI provisioning + test wiring

`.github/workflows/ci.yml` — in the integration-test job (the one with the Postgres
service + `THITTAM_TEST_DSN`), before running integration tests:
1. Create `thittam_app` (CI's `thittam` is superuser):
   `psql "$THITTAM_TEST_DSN" -c "CREATE ROLE thittam_app LOGIN PASSWORD 'thittam_app_ci';"`
   (guard with the `pg_roles` existence check for re-runs).
2. Run the grant SQL: `psql "$THITTAM_TEST_DSN" -f scripts/db-grant-app-role.sql`.
3. Export `THITTAM_TEST_APP_DSN` (same host/db, user `thittam_app`, password
   `thittam_app_ci`) so the proof test picks it up.

## Component 5 — proof integration test

New `pkg/audit/audit_role_integration_test.go` (`//go:build integration`). Reads
`THITTAM_TEST_APP_DSN`; **skips** when unset (so local/sandbox runs skip). Opens a pool
as `thittam_app` and asserts the privilege boundary:

- `INSERT INTO audit_log (...)` → **succeeds**.
- `SELECT FROM audit_log` → **succeeds**.
- `UPDATE audit_log SET ...` → **fails** with a permission error (SQLSTATE `42501`).
- `DELETE FROM audit_log` → **fails** with `42501`.
- Full CRUD on a normal table (insert then delete a throwaway `tenants` row, or a
  scratch table) → **succeeds**, proving the revoke is scoped to `audit_log` only.

Assert the error is specifically insufficient-privilege (pgconn `PgError.Code == "42501"`),
not any error, so a typo can't produce a false pass. Clean up inserted rows via the
owner connection (or the app role for the non-audit table).

## Acceptance criteria

- [ ] `thittam_app` created in the privileged init (local-db-init.sh) + CI, non-owner, least privilege.
- [ ] Grant SQL: schema/table/sequence grants + ALTER DEFAULT PRIVILEGES + `REVOKE UPDATE, DELETE ON audit_log`; idempotent; guarded on missing role.
- [ ] `make db-grant-app-role` target; wired into db-bootstrap + db-test-bootstrap.
- [ ] CI provisions `thittam_app`, runs the grant, exports `THITTAM_TEST_APP_DSN`.
- [ ] Proof test asserts INSERT/SELECT allowed, UPDATE/DELETE on audit_log denied (42501), non-audit CRUD allowed; skips when the role DSN is unset.
- [ ] **Verified in CI** (cannot run in this sandbox — no superuser). CI Lint/Integration green.

## Out of scope (deferred children)

- **#122** — split service DSNs so pods connect as `thittam_app` (the REVOKE only bites at runtime once this lands).
- **#123** — provision the prod `thittam_app` password + repoint the k8s Secret.

## Files touched

- `scripts/local-db-init.sh` (create thittam_app)
- `scripts/db-grant-app-role.sql` (new)
- `Makefile` (db-grant-app-role target)
- `scripts/db-bootstrap.sh`, `scripts/db-test-bootstrap.sh` (call the target post-migrate)
- `.github/workflows/ci.yml` (provision role + grant + export app DSN)
- `pkg/audit/audit_role_integration_test.go` (new proof test)

Review: DB-privilege/security change → senior review required. Note: real prod
enforcement is gated on #122 + #123.
