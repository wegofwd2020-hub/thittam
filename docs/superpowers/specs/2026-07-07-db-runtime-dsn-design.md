# Design: runtime DSN split — services connect as thittam_app (#122)

**Epic:** #120 · **This issue:** #122 · **Ops sibling:** #123 (prod Secret value).
**Date:** 2026-07-07
**Scope:** k8s manifest repoint + CI runs the integration suite under least privilege.
**Branch:** `feat/db-runtime-dsn-122`

## Context

#120 landed the `thittam_app` least-privilege role with `UPDATE`/`DELETE` revoked on
`audit_log`. But every service still connects as owner `thittam` (owner bypasses REVOKE),
so the enforcement is inert. This makes services connect as `thittam_app`:

- **Prod (deploy):** repoint the 11 k8s manifests from Secret key `url` (owner) to
  `runtime_url` (thittam_app). Migrations don't run in k8s — they run via `make migrate-all`
  as owner — so the cluster only needs the runtime DSN.
- **CI (proof):** run the whole integration suite connecting as `thittam_app`. This proves
  the grants are sufficient and the app works under least privilege — the assumption the
  prod repoint depends on.

### Verification reality

- **CI does not validate k8s YAML** (no kubeconform/kustomize/kubectl in any workflow). The
  manifest repoint is verifiable only by YAML syntax + deploy review.
- The prod repoint is **deploy-gated on #123**: `secretKeyRef` to a missing `runtime_url`
  key makes pods un-schedulable. Merging is harmless; deploying before #123 adds the key is
  not. Recorded as a loud manifest comment + PR note.
- The CI switch **is** verifiable — and will likely surface grant gaps, needing a few CI
  iterations fixing `scripts/db-grant-app-role.sql`. That iteration is the hardening value.

## Component 1 — prod manifest repoint

In each of these 11 files, change the `DATABASE_URL` `secretKeyRef` `key: url` → `key: runtime_url`:

`infra/k8s/services/{iam,project-management,budget-planning,expense-tracking,general-ledger,inventory-management,reporting-analytics,notifications,document,billing}.yaml`
and `infra/k8s/jobs/retention-sweeper-cronjob.yaml`.

Add a comment on each (or a shared note) referencing the deploy gate. Update the
`infra/k8s/config/configmap.yaml:11-12` doc block to describe both keys:
`url` (owner/migrator) and `runtime_url` (thittam_app runtime).

**Do NOT deploy before #123 adds `runtime_url` to the live `thittam-db` Secret.**

## Component 2 — CI: run the suite as thittam_app

In `.github/workflows/ci.yml`, the `integration-tests` job:
- Keep migrations on the **owner** DSN: the "Create thittam_test database" + migrate steps
  already use `TEST_DB_URL` (owner) — unchanged (thittam_app can't `CREATE TABLE`).
- Change the job-level `THITTAM_TEST_DSN` (what tests connect as) from the owner URL to the
  **thittam_app** URL: `postgres://thittam_app:thittam_app_ci@localhost:5432/thittam_test?sslmode=disable`.
- Add a new job-level env `THITTAM_TEST_OWNER_DSN` = the previous owner URL
  (`postgres://thittam:thittam_ci@localhost:5432/thittam_test?sslmode=disable`) — used by
  audit-log cleanups (thittam_app can't delete audit_log).
- The provisioning step (creates `thittam_app` + grants) already runs after migrations and
  before tests — unchanged. Nothing before it uses `THITTAM_TEST_DSN` (migrations use
  `TEST_DB_URL`), so pointing `THITTAM_TEST_DSN` at the not-yet-created role is safe.

Net: migrations as owner, tests as thittam_app, audit-log cleanup as owner.

## Component 3 — route audit_log cleanups through the owner DSN

With the test pool now `thittam_app`, the `DELETE FROM audit_log` cleanups must use an owner
connection. Sites (from grep):
- `pkg/audit/postgres_integration_test.go:24, :57` — currently `pool.Exec(... DELETE audit_log ...)` where `pool = testdb.Open` (now thittam_app). Route via owner.
- `services/iam/db/tenant_lifecycle_audit_integration_test.go:59` — `pool.Exec(... DELETE audit_log ...)`; the sibling `DELETE tenants` (:60) stays on the app pool (thittam_app has DELETE on tenants).
- `pkg/audit/audit_role_integration_test.go:45` — already deletes via an owner pool read from `THITTAM_TEST_DSN`; **rename that read to `THITTAM_TEST_OWNER_DSN`** (since `THITTAM_TEST_DSN` is now the app role). Its appPool (`THITTAM_TEST_APP_DSN`) is unchanged; the intentional deny-assertion delete at :66 stays on appPool.

Introduce a small helper for the owner-cleanup in `pkg/audit` tests (both audit tests are
`package audit_test`):

```go
// ownerExec runs a statement (e.g. audit_log cleanup) via the owner DSN, since
// thittam_app can't DELETE audit_log. No-op if THITTAM_TEST_OWNER_DSN is unset
// (local runs where the suite isn't split).
func ownerExec(ctx context.Context, sql string, args ...any) {
	dsn := os.Getenv("THITTAM_TEST_OWNER_DSN")
	if dsn == "" {
		return
	}
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return
	}
	defer p.Close()
	_, _ = p.Exec(ctx, sql, args...)
}
```

The iam/db lifecycle test gets an equivalent local helper (its own `package db_test`).

**Local behavior unchanged:** when `THITTAM_TEST_OWNER_DSN` is unset (local sandbox — no
role split), the owner helper no-ops, and the test pool is still the single owner DSN, so
audit_log cleanup happens on the same connection as before via the existing pool path. To
keep local cleanup working, the cleanup should: use the owner helper when
`THITTAM_TEST_OWNER_DSN` is set, else fall back to the existing `pool` delete. (Small
conditional, or: always attempt `pool` delete AND owner delete — the app-pool delete simply
errors-and-is-ignored under the split, and the owner delete no-ops locally. Prefer the
explicit conditional for clarity.)

## Component 4 — expect CI iteration for grant sufficiency

Running the full suite as `thittam_app` may reveal statements the grants don't cover
(a sequence, a table added post-grant, `TEMP`/`TRUNCATE`, etc.). Each surfaced gap is fixed
in `scripts/db-grant-app-role.sql` (re-run in CI's provisioning step). Anticipated-safe:
the grant already covers `SELECT/INSERT/UPDATE/DELETE ON ALL TABLES` + `USAGE,SELECT ON ALL
SEQUENCES` + `ALTER DEFAULT PRIVILEGES`; `TEMP` is granted to `PUBLIC` by default. Watch for
`TRUNCATE` (owner-only) in tests.

## Acceptance criteria

- [ ] 11 manifests repointed to `key: runtime_url`; configmap doc updated; deploy-gate note present.
- [ ] CI: migrations as owner (`TEST_DB_URL`), tests as `thittam_app` (`THITTAM_TEST_DSN`), `THITTAM_TEST_OWNER_DSN` added for cleanup.
- [ ] audit_log cleanups routed through the owner DSN; local (unsplit) runs still clean up correctly.
- [ ] **CI integration suite green while connected as `thittam_app`** — the proof the app works under least privilege (grant gaps fixed as surfaced).

## Out of scope

- **#123** — provision the prod `thittam_app` password + add `runtime_url` to the live Secret. Deploy of #122 is gated on it.
- Switching local `dev-start.sh` to thittam_app (optional follow-on; role exists locally via #120's local-db-init but not wired).

## Files touched

- `infra/k8s/services/*.yaml` (10) + `infra/k8s/jobs/retention-sweeper-cronjob.yaml` (key repoint)
- `infra/k8s/config/configmap.yaml` (doc)
- `.github/workflows/ci.yml` (DSN env split)
- `pkg/audit/postgres_integration_test.go`, `pkg/audit/audit_role_integration_test.go`, `services/iam/db/tenant_lifecycle_audit_integration_test.go` (owner cleanup)
- possibly `scripts/db-grant-app-role.sql` (grant gaps, as CI surfaces them)

Review: DB-privilege/infra change → senior review. Deploy gated on #123.
