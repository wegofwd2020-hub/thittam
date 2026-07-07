# Least-privilege thittam_app role — audit_log append-only (#120) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.
>
> **⚠️ Verification is CI-only.** This sandbox's `thittam` is not a superuser and cannot `CREATE ROLE`; `sudo` hangs. So the role can't be provisioned locally and the proof test **skips** locally. The real gate is CI (its Postgres `thittam` is a superuser). Validate locally only what's possible: SQL/YAML/shell well-formedness and `go build`/`go vet`. Push and watch CI.

**Goal:** Land the enforcement primitive for audit_log append-only — a non-owner `thittam_app` role with least privilege and `UPDATE`/`DELETE` revoked on `audit_log`, proven by an integration test. Service DSN rewire (#122) + prod secret (#123) deferred.

**Architecture:** Role creation is privileged (local-db-init.sh sudo path + a CI step, since `CREATE ROLE` needs superuser). Grants/revoke run as owner `thittam` via a post-migrate SQL script. A proof integration test connects AS `thittam_app` and asserts the privilege boundary.

## Global Constraints

- `CREATE ROLE` needs superuser/CREATEROLE → role creation only in privileged contexts (local-db-init sudo path; CI where `thittam` is superuser). GRANT/REVOKE run as owner.
- Grant SQL must be idempotent (`GRANT`/`REVOKE`/`ALTER DEFAULT PRIVILEGES` are) and guard on a missing role.
- Proof test asserts insufficient-privilege specifically (pgconn `PgError.Code == "42501"`), not any error; skips when `THITTAM_TEST_APP_DSN` unset.
- CI Postgres service: `POSTGRES_USER: thittam` (superuser), DSN `postgres://thittam:thittam_ci@localhost:5432/thittam_test`.

---

### Task 1: Role creation + grant/revoke SQL + local wiring

**Files:**
- Create: `scripts/db-grant-app-role.sql`
- Modify: `scripts/local-db-init.sh` (create thittam_app)
- Modify: `Makefile` (db-grant-app-role target)
- Modify: `scripts/db-bootstrap.sh`, `scripts/db-test-bootstrap.sh` (call target post-migrate)

**Interfaces:**
- Produces: role `thittam_app` (created in privileged contexts); `make db-grant-app-role`; `scripts/db-grant-app-role.sql`.

- [ ] **Step 1: Write the grant/revoke SQL**

Create `scripts/db-grant-app-role.sql`:

```sql
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
```

- [ ] **Step 2: Add thittam_app creation to local-db-init.sh**

In `scripts/local-db-init.sh`, after the `thittam` role block and near the top add the
app-role vars, then after the database creation add an idempotent app-role block
(same `sudo -u postgres` pattern as the existing role creation):

```bash
DB_APP_USER=${DB_APP_USER:-thittam_app}
DB_APP_PASS=${DB_APP_PASS:-thittam_app_dev}
```

```bash
echo "--> Creating app role '$DB_APP_USER'..."
sudo -u postgres psql -p "$PG_PORT" -tc \
  "SELECT 1 FROM pg_roles WHERE rolname='$DB_APP_USER'" | grep -q 1 \
  && echo "    App role already exists — skipping." \
  || sudo -u postgres psql -p "$PG_PORT" -c \
       "CREATE ROLE $DB_APP_USER LOGIN PASSWORD '$DB_APP_PASS';" \
  && echo "    App role created."
```

`thittam_app` gets no CREATEDB/CREATEROLE/SUPERUSER and owns nothing — only the
explicit grants from Step 1.

- [ ] **Step 3: Add the Makefile target**

In `Makefile`, near the other `db-*` targets:

```make
# db-grant-app-role — grant thittam_app least privilege + revoke UPDATE/DELETE on
# audit_log. Run as owner after migrate-all. Role must already exist.
db-grant-app-role:
	psql "$(DB_URL)" -v ON_ERROR_STOP=1 -f scripts/db-grant-app-role.sql
```

Add `db-grant-app-role` to the `.PHONY` list if the Makefile maintains one (grep `.PHONY`).

- [ ] **Step 4: Wire into the bootstrap scripts**

In `scripts/db-bootstrap.sh` and `scripts/db-test-bootstrap.sh`, after the `make
migrate-all ...` line, add the grant step against the same DSN. Make it non-fatal in the
local scripts (a dev who hasn't re-run local-db-init won't have the role yet — warn, don't
abort the whole bootstrap):

```bash
make db-grant-app-role DB_URL="$DB_URL" || echo "WARN: db-grant-app-role skipped (is thittam_app created? run local-db-init.sh)"
```

(Use the script's actual DSN var — `$DB_URL` in db-test-bootstrap.sh.)

- [ ] **Step 5: Local validation (limited — role can't be created here)**

Run: `bash -n scripts/local-db-init.sh && bash -n scripts/db-bootstrap.sh && bash -n scripts/db-test-bootstrap.sh` (shell parse check).
Run: `make -n db-grant-app-role` (target resolves).
Run (expected to FAIL cleanly with the guard, proving the SQL parses + the guard fires, since thittam_app doesn't exist in this sandbox):
`psql "postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" -v ON_ERROR_STOP=1 -f scripts/db-grant-app-role.sql`
Expected: ERROR `role thittam_app does not exist — create it first ...` (the `DO` guard) — this confirms the SQL is syntactically valid and the guard works; it does NOT prove the grants (needs the role, which requires superuser).

- [ ] **Step 6: Commit**

```bash
git add scripts/db-grant-app-role.sql scripts/local-db-init.sh Makefile \
        scripts/db-bootstrap.sh scripts/db-test-bootstrap.sh
git commit -m "feat(db): thittam_app least-privilege role + audit_log REVOKE (#120)"
```

---

### Task 2: CI provisioning + proof integration test

**Files:**
- Modify: `.github/workflows/ci.yml` (integration-tests job: provision role, grant, export app DSN)
- Test: `pkg/audit/audit_role_integration_test.go` (new)

**Interfaces:**
- Consumes: `scripts/db-grant-app-role.sql` (Task 1), `THITTAM_TEST_DSN` (owner, CI), `THITTAM_TEST_APP_DSN` (thittam_app, exported by CI).

- [ ] **Step 1: Add CI provisioning steps**

In `.github/workflows/ci.yml`, in the `integration-tests` job (starts ~:203), AFTER the
migrate-up step (~:249-256) and BEFORE "Run integration tests" (~:260), add:

```yaml
      - name: Provision thittam_app role + grants
        run: |
          psql "$THITTAM_TEST_DSN" -v ON_ERROR_STOP=1 -c \
            "DO \$\$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='thittam_app') THEN CREATE ROLE thittam_app LOGIN PASSWORD 'thittam_app_ci'; END IF; END \$\$;"
          psql "$THITTAM_TEST_DSN" -v ON_ERROR_STOP=1 -f scripts/db-grant-app-role.sql
          echo "THITTAM_TEST_APP_DSN=postgres://thittam_app:thittam_app_ci@localhost:5432/thittam_test?sslmode=disable" >> "$GITHUB_ENV"
```

Confirm `psql` is available on the runner (the job already uses the golang-migrate CLI;
`postgresql-client` may need an `apt-get install -y postgresql-client` line at the top of
this step if `psql` isn't preinstalled — check the runner image / add if the step fails).
The `>> $GITHUB_ENV` export makes `THITTAM_TEST_APP_DSN` visible to the later test step.

- [ ] **Step 2: Write the proof integration test**

Create `pkg/audit/audit_role_integration_test.go`:

```go
//go:build integration

package audit_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditLog_AppRole_AppendOnly proves the DB-level append-only enforcement:
// thittam_app can INSERT/SELECT audit_log but NOT UPDATE/DELETE it. Requires the
// thittam_app DSN (provisioned in CI); skips locally where the role doesn't exist.
func TestAuditLog_AppRole_AppendOnly(t *testing.T) {
	appDSN := os.Getenv("THITTAM_TEST_APP_DSN")
	if appDSN == "" {
		t.Skip("THITTAM_TEST_APP_DSN not set — thittam_app role not provisioned (CI-only)")
	}
	ctx := context.Background()

	appPool, err := pgxpool.New(ctx, appDSN)
	require.NoError(t, err)
	defer appPool.Close()

	tenant := uuid.New()
	// Cleanup via the OWNER DSN — thittam_app can't DELETE audit_log (that's the point).
	t.Cleanup(func() {
		ownerDSN := os.Getenv("THITTAM_TEST_DSN")
		if ownerDSN == "" {
			return
		}
		op, err := pgxpool.New(ctx, ownerDSN)
		if err != nil {
			return
		}
		defer op.Close()
		_, _ = op.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
	})

	// INSERT allowed.
	_, err = appPool.Exec(ctx, `
		INSERT INTO audit_log (tenant_id, actor_id, actor_email, action, resource_type, resource_id)
		VALUES ($1, $2, 'system:test', 'status_changed', 'tenant', $1)`,
		tenant, uuid.Nil)
	require.NoError(t, err, "thittam_app must be able to INSERT audit_log")

	// SELECT allowed.
	var n int
	require.NoError(t, appPool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1`, tenant).Scan(&n))
	assert.Equal(t, 1, n)

	// UPDATE denied (42501 insufficient_privilege).
	_, err = appPool.Exec(ctx, `UPDATE audit_log SET action = 'tampered' WHERE tenant_id = $1`, tenant)
	assertInsufficientPrivilege(t, err, "UPDATE audit_log")

	// DELETE denied.
	_, err = appPool.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
	assertInsufficientPrivilege(t, err, "DELETE audit_log")
}

func assertInsufficientPrivilege(t *testing.T, err error, op string) {
	t.Helper()
	require.Error(t, err, "%s must be denied for thittam_app", op)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "%s: expected pgconn.PgError, got %T", op, err)
	assert.Equal(t, "42501", pgErr.Code, "%s: expected insufficient_privilege (42501), got %s", op, pgErr.Code)
}
```

Note: the non-audit-CRUD-allowed assertion from the spec is omitted to avoid coupling to
another table's schema/constraints; the INSERT-allowed + UPDATE/DELETE-denied trio already
proves the revoke is scoped to `audit_log` (the blanket GRANT covers other tables). If the
reviewer wants the non-audit proof, add an insert+delete against a scratch temp table
created by the app role.

- [ ] **Step 3: Local validation**

Run: `go vet ./pkg/audit/... && go build -tags=integration ./pkg/audit/...`
Run: `THITTAM_TEST_DSN="postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" go test -tags=integration ./pkg/audit/ -run TestAuditLog_AppRole_AppendOnly -v`
Expected: the test **SKIPS** (`THITTAM_TEST_APP_DSN not set`) — confirms it compiles and skips cleanly locally. Real assertion happens in CI.

- [ ] **Step 4: Validate the CI YAML**

Run (if `yamllint`/`actionlint` available): `actionlint .github/workflows/ci.yml` or `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))"` (YAML well-formedness).
Expected: no parse errors.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml pkg/audit/audit_role_integration_test.go
git commit -m "test(db): CI-provision thittam_app + prove audit_log append-only (#120)"
```

- [ ] **Step 6: Push + watch CI (the actual verification)**

After both tasks: push and watch the `integration-tests` job. The proof test must run
(not skip) in CI and PASS — INSERT/SELECT ok, UPDATE/DELETE denied with 42501. If the
provisioning step fails (e.g. `psql` missing), add `apt-get install -y postgresql-client`.

---

## Self-Review

**Spec coverage:**
- thittam_app created privileged (local-db-init + CI) → Task 1 Step 2, Task 2 Step 1. ✅
- Grant SQL (grants + ALTER DEFAULT + REVOKE audit_log) idempotent + guarded → Task 1 Step 1. ✅
- Makefile target + bootstrap wiring → Task 1 Steps 3-4. ✅
- CI provisions role + grant + exports app DSN → Task 2 Step 1. ✅
- Proof test (INSERT/SELECT ok, UPDATE/DELETE 42501, skips when unset) → Task 2 Step 2. ✅
- CI-verified acknowledgment → header + Task 2 Step 6. ✅
- Out of scope (#122 DSN split, #123 prod secret) — not in plan. ✅

**Consistency:** role name `thittam_app`, password `thittam_app_dev` (local) / `thittam_app_ci` (CI), `THITTAM_TEST_APP_DSN` env — consistent across local-db-init, ci.yml, and the test. Grant SQL is the single source referenced by both the Makefile target and the CI step.

**Placeholder scan:** No TBD. The `apt-get install postgresql-client` and `actionlint` steps are explicitly conditional ("if the step fails" / "if available") — real fallbacks, not vague. The intentionally-omitted non-audit-CRUD assertion is called out with the rationale and how to add it if review wants it.

**Verification honesty:** every step states what it can and cannot prove locally; the plan header and Task 2 Step 6 make CI the explicit gate. Given no local test loop, **inline execution (or close controller review) fits better than fully-autonomous subagents** — the executor should push early and read CI, not chase local green.
