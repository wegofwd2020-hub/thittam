# thittam_app prod cutover — repo-side support (#123) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-29
**Issue:** #123 (ops(db): provision thittam_app password + repoint prod Secret) — child of epic #120, sibling of #122 (DSN split, MERGED)
**Branch:** `chore/app-role-prod-cutover-123` off `main` (0b5c22b)
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

#123 is an **operations action against production** (create the prod `thittam_app` role with a real
password, grant least privilege, add a `runtime_url` key to the live `thittam-db` k8s Secret, roll the
deployments, verify append-only enforcement). It cannot be executed from this repo — there is no prod
cluster/DB/secret access here, and by design no Secret object is committed.

What this repo CAN and should provide, to make that manual cutover **repeatable, reviewed, and
secret-safe**, are two artifacts:

1. `scripts/prod-provision-app-role.sql` — a parameterized, idempotent CREATE-or-set-password for the
   prod role, with the password supplied via a psql variable so it never enters git.
2. `docs/operations/app-role-prod-cutover.md` — an ordered runbook for the whole cutover (preflight →
   provision → grant → secret → roll → verify → rollback), with the exact commands.

## Context (grounding facts, `main` @ 0b5c22b)

- **Epic #120** has three legs: SQL foundation (role bootstrap + grants + REVOKE), **#122 DSN split
  (MERGED)**, and **#123 prod ops (this)**. Append-only enforcement takes effect in prod only once all
  three land. #122 already repointed the manifests; #124/#125 shipped the role SQL + service DSN wiring.
- **`scripts/db-grant-app-role.sql`** (exists): GRANTs least privilege to `thittam_app`
  (`SELECT,INSERT,UPDATE,DELETE ON ALL TABLES`, `USAGE,SELECT ON ALL SEQUENCES`, `ALTER DEFAULT
  PRIVILEGES`), then `REVOKE UPDATE, DELETE ON audit_log FROM thittam_app`. It **does NOT create the
  role** — a `DO $$` guard raises if `thittam_app` is absent. Run as owner (`thittam`) after migrations,
  via `make db-grant-app-role DB_URL=…` (`Makefile`: `psql "$(DB_URL)" -v ON_ERROR_STOP=1 -f
  scripts/db-grant-app-role.sql`).
- **`CREATE ROLE thittam_app LOGIN PASSWORD …`** exists ONLY in non-prod, superuser contexts:
  `scripts/local-db-init.sh` (dev, password `thittam_app_dev`) and `.github/workflows/ci.yml` (CI,
  password `thittam_app_ci`). **No prod-reachable CREATE ROLE / password-setting script exists** — the
  gap #123 covers.
- **DSN model:** every service reads a single `DATABASE_URL` env var; there is no `pkg/db` and no
  code-level owner/runtime split. The split is purely which Secret **key** feeds that env var. All 10
  service manifests (`infra/k8s/services/{billing,budget-planning,document,expense-tracking,
  general-ledger,iam,inventory-management,notifications,project-management,reporting-analytics}.yaml`)
  and `infra/k8s/jobs/retention-sweeper-cronjob.yaml` already use
  `secretKeyRef{name: thittam-db, key: runtime_url}`. **`infra/k8s/jobs/purge-worker-cronjob.yaml`
  deliberately stays on `key: url`** (owner — purge needs `DROP SCHEMA`, which `thittam_app` cannot do).
- **The `thittam-db` Secret is a bare cluster object** — no manifest of any kind in the repo (no literal,
  SealedSecret, SOPS, or ExternalSecret). `infra/k8s/config/configmap.yaml` documents the expected shape
  and carries the load-bearing warning: *"DO NOT roll out this repoint until the thittam-db Secret
  actually has a `runtime_url` key (#123) — a missing key makes pods un-schedulable."* Because the
  manifests are already repointed in git but #123 isn't done, the correct prod state is that the
  repointed manifests have **not yet been applied** to prod (or prod still runs pre-repoint pods) — the
  runbook must not assume otherwise.
- **audit_log append-only** is privilege-based only (GRANT + targeted REVOKE in the grant script); no
  trigger/rule. The commented `-- REVOKE …` pointer in `migrations/audit/001_create_audit_log.up.sql`
  is intentionally inert (the live REVOKE lives in the grant script) — leave it (out of scope).

## Design

### Artifact A — `scripts/prod-provision-app-role.sql`

Idempotent, secret-safe, run as a **superuser** against the prod owner/admin DSN. The password is a psql
variable (`-v app_password=…`); `:'app_password'` produces a properly quoted+escaped literal and never
appears in git or in `\dp`/logs beyond the role definition.

```sql
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
```

Notes:
- `\if :{?app_password}` tests whether the variable is set; missing → `\warn` + `\quit` (non-empty exit
  is not guaranteed by `\quit`, but `ON_ERROR_STOP` plus the absent role in later steps fails safe — the
  runbook also greps output). The guard's job is a clear operator message, not a hard gate.
- `\gset` captures the existence check into `need_create`; the two branches are the only place a
  password is written, and both go through `:'app_password'` (quoted).
- CREATE grants NO privileges — a fresh `thittam_app` can log in but touch nothing until
  `db-grant-app-role.sql` runs. That ordering matches the existing dev/CI flow.

### Artifact B — `docs/operations/app-role-prod-cutover.md`

New file (this repo has no `docs/operations/` yet; create it). The formal service runbook referenced by
alerts lives in `thittam_docs`, but this cutover references repo-local scripts and manifests, so it
belongs beside them. Ordered sections, each with copy-pasteable commands and the placeholders an
operator fills:

1. **Preflight / safety.** State the invariant from `configmap.yaml`: service manifests already point at
   `key: runtime_url`, so the `runtime_url` Secret key must exist **before** those manifests are applied
   or pods go un-schedulable. Confirm: (a) you hold a prod **superuser** DSN (for CREATE ROLE) and the
   **owner** (`thittam`) DSN (for grants); (b) prod is either still on pre-repoint pods or not yet
   deployed with the repointed manifests. Command to check what key the running Deployments reference.
2. **Provision the role.** Read the new password from the secret manager into a shell var (never echo),
   run `scripts/prod-provision-app-role.sql` with `-v app_password=…` against the superuser DSN.
3. **Grant least privilege.** `make db-grant-app-role DB_URL="$PROD_OWNER_DSN"` (owner DSN) — applies
   GRANTs + `REVOKE UPDATE, DELETE ON audit_log`. Must run after migrations (all prod tables exist).
4. **Add `runtime_url` to the Secret.** Patch the live `thittam-db` Secret to add the `runtime_url` key
   (`postgres://thittam_app:<password>@postgres.thittam.svc.cluster.local:5432/thittam`) while
   preserving `url`. Show the `kubectl create secret … --dry-run=client -o yaml | kubectl apply -f -`
   merge pattern (or `kubectl patch` with a base64 value), noting the password must match step 2.
5. **Roll the workloads.** `kubectl rollout restart deploy` for the 10 services; note the two CronJobs
   pick up the new Secret on their **next scheduled run** (no rollout), and that `purge-worker` stays on
   `url` intentionally.
6. **Verify enforcement.** Connect as `thittam_app` (runtime DSN) and prove append-only: an `INSERT`
   into `audit_log` succeeds; an `UPDATE` and a `DELETE` both fail with `permission denied for table
   audit_log`. Also confirm a normal service read/write path works.
7. **Rollback.** If `thittam_app` cannot connect or a service breaks: emergency-repoint the affected
   Deployments' `secretKeyRef.key` back to `url` (owner) and `rollout restart` to restore service, then
   diagnose. The role/grant steps are idempotent and safe to re-run.

The runbook links #120/#122/#123 and the two scripts, and ends with a checklist an operator ticks off.

## Testing

- **Artifact A:** validate the SQL runs against a **disposable, uniquely-named throwaway Postgres
  container** (NEVER `docker compose … -v/down/up` against `infra/local/` — that destroys shared
  volumes). Prove: (1) with `-v app_password=…` on a DB where the role is absent → role created, can
  `\du` show it LOGIN; (2) re-run → takes the ALTER branch, no error (idempotent); (3) run WITHOUT
  `-v app_password` → prints the guard message and does not create a passwordless role. Because this is
  operator SQL, not Go, it has no unit test; the throwaway-container check is the gate. If a throwaway
  container isn't available in the sandbox, document that the SQL was lint-read only and mark the
  container check as the reviewer's / operator's responsibility.
- **Artifact B:** prose only — reviewer checks the command sequence is correct, the ordering respects the
  "runtime_url before apply" invariant, the DSNs used per step are right (superuser for CREATE, owner for
  grants, runtime for verify), and no secret is ever echoed or committed.
- **No** Go build/test/migration/proto/sqlc impact — the branch adds two files under `scripts/` and
  `docs/` and touches nothing the services compile against. `go build ./...` remains green trivially.

## Non-goals

- Executing any prod step (CREATE ROLE, secret patch, rollout) — no prod access; the operator runs them.
- Committing a `thittam-db` Secret object in any form — creds stay out of git (bare `kubectl`).
- Changing the already-correct manifests (services on `runtime_url`, purge-worker on `url`) or the
  commented REVOKE pointer in `migrations/audit/001` (#124's decision).
- A Vault/SOPS/SealedSecret workflow — the chosen mechanism is a bare `kubectl` Secret.
- Closing epic #120 — that flips to done when the operator completes this runbook in prod; this branch
  only delivers the tooling + runbook.

## Review weight

Low code risk (no Go, no CI-gated artifact). The review that matters is **operational correctness**: the
psql control-flow + idempotency of Artifact A, and the step ordering / DSN-per-step / secret-hygiene of
Artifact B. A security-minded reviewer should confirm no path writes a password to git, logs, or a
world-readable place, and that the append-only verification step actually proves the REVOKE.
