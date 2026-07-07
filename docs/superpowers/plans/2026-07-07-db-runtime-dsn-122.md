# Runtime DSN split — services as thittam_app (#122) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans. Steps use checkbox (`- [ ]`).
>
> **⚠️ Verification is CI-only for the enforcement.** The sandbox has no `thittam_app` role, so the "suite runs as thittam_app" proof only happens in CI. Locally, the owner-cleanup helper falls back to the single pool, so local integration tests still pass unchanged. k8s manifests are unvalidated by CI — YAML syntax + deploy review only.

**Goal:** Make services connect as least-privilege `thittam_app` so #120's audit_log REVOKE actually enforces. Repoint 11 k8s manifests to the `runtime_url` Secret key; run the CI integration suite as `thittam_app` to prove the grants suffice.

## Global Constraints

- Migrations run as **owner** (`thittam`) — `thittam_app` can't `CREATE TABLE`. Only the runtime/test connection switches.
- audit_log cleanups must route through an **owner** DSN (`thittam_app` can't DELETE audit_log — that's the enforcement).
- Local (unsplit) runs: `THITTAM_TEST_OWNER_DSN` unset → cleanup helper falls back to the existing pool; behavior unchanged.
- Prod manifest repoint is **deploy-gated on #123** (Secret must have `runtime_url` first). Merge is safe; deploy alone is not.

---

### Task 1: CI runs the suite as thittam_app + audit_log cleanup owner-routing

**Files:**
- Modify: `.github/workflows/ci.yml` (integration-tests job env)
- Modify: `pkg/audit/postgres_integration_test.go` (2 cleanups)
- Modify: `pkg/audit/audit_role_integration_test.go` (owner cleanup rename)
- Modify: `services/iam/db/tenant_lifecycle_audit_integration_test.go` (audit_log cleanup)

- [ ] **Step 1: Split the CI DSN env**

In `.github/workflows/ci.yml`, the `integration-tests` job env (:222-226), change
`THITTAM_TEST_DSN` to the app role and add the owner DSN:

```yaml
    env:
      # Tests run against a dedicated thittam_test DB so they cannot collide
      # with a parallel migration-validate job using thittam.
      TEST_DB_URL: postgres://thittam:thittam_ci@localhost:5432/thittam_test?sslmode=disable
      # Tests connect as the least-privilege runtime role (#122) so the audit_log
      # REVOKE (#120) is actually exercised. Migrations still use TEST_DB_URL (owner).
      THITTAM_TEST_DSN: postgres://thittam_app:thittam_app_ci@localhost:5432/thittam_test?sslmode=disable
      # Owner DSN for audit_log cleanup (thittam_app can't DELETE audit_log).
      THITTAM_TEST_OWNER_DSN: postgres://thittam:thittam_ci@localhost:5432/thittam_test?sslmode=disable
```

(The "Create thittam_test database" + migrate steps use `TEST_DB_URL` — unchanged. The
provisioning step that creates `thittam_app` runs before the test step; nothing before it
uses `THITTAM_TEST_DSN`.) Also the provisioning step's exported `THITTAM_TEST_APP_DSN` now
equals `THITTAM_TEST_DSN` — leave it (the proof test reads it).

- [ ] **Step 2: Add the owner-cleanup helper in pkg/audit tests**

Add to `pkg/audit/postgres_integration_test.go` (package `audit_test`), near the top after
imports:

```go
// cleanupAuditLog removes a tenant's audit_log rows. Under the least-privilege
// suite (THITTAM_TEST_OWNER_DSN set, tests connect as thittam_app which can't
// DELETE audit_log) it deletes via the owner DSN; otherwise it uses the given
// pool (local single-role runs).
func cleanupAuditLog(ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID) {
	if dsn := os.Getenv("THITTAM_TEST_OWNER_DSN"); dsn != "" {
		if op, err := pgxpool.New(ctx, dsn); err == nil {
			defer op.Close()
			_, _ = op.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
			return
		}
	}
	_, _ = pool.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
}
```

Ensure imports include `os` and `github.com/jackc/pgx/v5/pgxpool` (pgxpool may be new to
this file).

- [ ] **Step 3: Route the two postgres_integration_test cleanups through the helper**

Replace both cleanup bodies (`:24`, `:57` — identical `_, _ = pool.Exec(... DELETE audit_log ...)`) with:

```go
	t.Cleanup(func() { cleanupAuditLog(context.Background(), pool, tenant) })
```

- [ ] **Step 4: Fix the proof test's owner cleanup**

In `pkg/audit/audit_role_integration_test.go`, the `t.Cleanup` (~:37-47) currently reads
`THITTAM_TEST_DSN` for the owner connection — but that's now the app role. Replace its body
with the shared helper (which reads `THITTAM_TEST_OWNER_DSN`):

```go
	t.Cleanup(func() { cleanupAuditLog(ctx, appPool, tenant) })
```

(The helper prefers `THITTAM_TEST_OWNER_DSN`, always set in CI where this test runs; the
`appPool` fallback is never hit here since the test skips when unsplit.) Remove the now-unused
inline owner-pool cleanup code and any now-unused imports.

- [ ] **Step 5: Fix the iam/db lifecycle test cleanup**

In `services/iam/db/tenant_lifecycle_audit_integration_test.go` (`package db_test`), the
cleanup (:58-61) deletes audit_log (:59) then tenants (:60). Route the **audit_log** delete
through the owner DSN; keep the **tenants** delete on the pool (thittam_app has DELETE on
tenants). Add a local helper (this package can't see pkg/audit's helper):

```go
func ownerDeleteAuditLog(ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID) {
	if dsn := os.Getenv("THITTAM_TEST_OWNER_DSN"); dsn != "" {
		if op, err := pgxpool.New(ctx, dsn); err == nil {
			defer op.Close()
			_, _ = op.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
			return
		}
	}
	_, _ = pool.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
}
```

and in the cleanup:

```go
	t.Cleanup(func() {
		ownerDeleteAuditLog(ctx, pool, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})
```

Add `os` + `pgxpool` imports if missing.

- [ ] **Step 6: Local validation (fallback path)**

Run: `go vet ./... && go build -tags=integration ./pkg/audit/... ./services/iam/db/...`
Run (unset OWNER_DSN → helper falls back to pool, local single-role behavior unchanged):
`THITTAM_TEST_DSN="postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" go test -tags=integration ./pkg/audit/ ./services/iam/db/ -run 'TestPostgresAudit|TestTenantLifecycle_EmitsAudit' 2>&1 | tail`
Expected: PASS (the audit tests still run as owner locally; helper no-ops OWNER_DSN and deletes via pool). `TestAuditLog_AppRole_AppendOnly` still SKIPS locally.

- [ ] **Step 7: Commit**

```bash
git add .github/workflows/ci.yml pkg/audit/postgres_integration_test.go \
        pkg/audit/audit_role_integration_test.go \
        services/iam/db/tenant_lifecycle_audit_integration_test.go
git commit -m "test(db): run CI integration suite as thittam_app; route audit_log cleanup via owner (#122)"
```

- [ ] **Step 8: Push + watch CI — iterate on grant gaps**

Push. The `integration-tests` job now connects as `thittam_app`. If any test fails with
`42501` (insufficient privilege) on a legitimate operation, add the missing GRANT to
`scripts/db-grant-app-role.sql` and re-push. Repeat until the suite is green as thittam_app.
Watch for `TRUNCATE` (owner-only) — if a test truncates, either grant is impossible (route
that op through owner like the audit cleanups) or the test needs adjustment.

---

### Task 2: Repoint prod manifests to runtime_url

**Files:** the 11 manifests + configmap.

- [ ] **Step 1: Repoint all 11 manifests**

In each file, change the `DATABASE_URL` `secretKeyRef` `key: url` → `key: runtime_url`:
`infra/k8s/services/iam.yaml`, `project-management.yaml`, `budget-planning.yaml`,
`expense-tracking.yaml`, `general-ledger.yaml`, `inventory-management.yaml`,
`reporting-analytics.yaml`, `notifications.yaml`, `document.yaml`, `billing.yaml`,
and `infra/k8s/jobs/retention-sweeper-cronjob.yaml`.

Sanity after: `grep -rn "key: url" infra/k8s/` should return **nothing**;
`grep -rln "key: runtime_url" infra/k8s/ | wc -l` should be 11.

- [ ] **Step 2: Update the configmap doc + add deploy-gate note**

In `infra/k8s/config/configmap.yaml:11-12`, expand the `thittam-db` Secret doc to describe
both keys:

```yaml
#   thittam-db
#     url:          postgres://thittam:<password>@postgres.../thittam       (owner — migrations, admin)
#     runtime_url:  postgres://thittam_app:<password>@postgres.../thittam   (least-privilege runtime; #120/#122)
#   NOTE: service pods use runtime_url. Do NOT roll out the runtime_url repoint
#   until the thittam-db Secret actually has a runtime_url key (#123).
```

- [ ] **Step 3: Validate YAML**

Run: `for f in $(git diff --name-only | grep infra/k8s); do python3 -c "import yaml,sys; list(yaml.safe_load_all(open('$f')))" && echo "OK $f"; done`
Expected: all OK (well-formed). No runtime validation possible (CI doesn't check manifests).

- [ ] **Step 4: Commit**

```bash
git add infra/k8s/
git commit -m "infra(db): repoint service pods to runtime_url (thittam_app) Secret key (#122)"
```

---

## Self-Review

**Spec coverage:**
- 11 manifests → runtime_url + configmap doc + deploy-gate → Task 2. ✅
- CI: migrations owner, tests thittam_app, THITTAM_TEST_OWNER_DSN added → Task 1 Step 1. ✅
- audit_log cleanups via owner; local fallback → Task 1 Steps 2-5. ✅
- CI suite green as thittam_app (iterate grants) → Task 1 Step 8. ✅
- Out of scope (#123 prod secret, dev-start.sh) — not in plan. ✅

**Consistency:** `THITTAM_TEST_OWNER_DSN` used identically by both helpers (pkg/audit `cleanupAuditLog`, iam/db `ownerDeleteAuditLog`) and the CI env. `runtime_url` key name consistent across manifests + configmap doc. Migrations stay on `TEST_DB_URL` (owner).

**Placeholder scan:** No TBD. Task 1 Step 8's grant-gap iteration is an explicit push-watch-fix loop with concrete failure signatures (`42501`, `TRUNCATE`), not vague. The local-fallback path is spelled out so local runs stay green.

**Verification honesty:** every step states local vs CI reachability; the enforcement proof (suite as thittam_app) is explicitly CI-only and may iterate. Inline execution fits (no local proof loop) — push early, read CI.
