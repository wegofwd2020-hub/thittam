# thittam_app prod cutover support (#123) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the two repo-side artifacts that make the #123 prod `thittam_app` cutover repeatable and secret-safe: a parameterized provisioning SQL script and an ordered operations runbook.

**Architecture:** Pure ops tooling — no Go, no CI-gated code. Task 1 adds an idempotent, password-via-psql-variable `CREATE`/`ALTER ROLE` SQL script and proves it against a disposable throwaway Postgres container. Task 2 adds the cutover runbook that sequences the manual prod steps (provision → grant → secret → roll → verify → rollback). The actual prod execution stays with the operator.

**Tech Stack:** psql client-side control flow (`\if`, `\gset`, `\quit`), Postgres roles/privileges, kubectl, Markdown.

## Global Constraints

- Module path `github.com/wegofwd2020/thittam`. No proto/sqlc/migration/Go changes; `go build ./...` stays green trivially.
- **No secret ever enters git, logs, or a world-readable place.** The password is supplied at run time via `psql -v app_password=…`; the script uses `:'app_password'` (auto-quoted). No literal password in either artifact.
- **DB safety:** NEVER run `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes (destroyed unrelated MinIO data once). Validate SQL only against a **disposable, uniquely-named throwaway container** you create and remove yourself.
- The script does NOT grant privileges — least-privilege GRANTs + the `audit_log` REVOKE stay in the existing `scripts/db-grant-app-role.sql` (run separately as owner). Do not duplicate them.
- Manifests are already correct (10 services + retention-sweeper on `key: runtime_url`; `purge-worker` on `key: url`). Do NOT change them. Do NOT touch the commented REVOKE pointer in `migrations/audit/001_create_audit_log.up.sql`.
- Commits end with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

### Task 1: Parameterized provisioning script `scripts/prod-provision-app-role.sql`

**Files:**
- Create: `scripts/prod-provision-app-role.sql`
- Test: manual — a disposable throwaway Postgres container (no Go test; this is operator SQL)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: the script path + its invocation contract, which Task 2's runbook references verbatim:
  `psql "$PROD_SUPERUSER_DSN" -v ON_ERROR_STOP=1 -v app_password="…" -f scripts/prod-provision-app-role.sql`.

**Context:** `CREATE ROLE thittam_app LOGIN PASSWORD …` today exists only in dev (`scripts/local-db-init.sh`, password `thittam_app_dev`) and CI (`.github/workflows/ci.yml`, password `thittam_app_ci`). No prod-reachable variant exists. This script fills that gap: idempotent (create if absent, else reset password), password from a psql variable so it never lands in git. It grants NOTHING — a fresh `thittam_app` can log in but touch nothing until `scripts/db-grant-app-role.sql` runs as owner. `:'app_password'` produces a correctly quoted/escaped literal; `\if :{?app_password}` tests whether the variable is set.

- [ ] **Step 1: Write the script**

Create `scripts/prod-provision-app-role.sql` with exactly this content:

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

- [ ] **Step 2: Spin up a disposable throwaway Postgres container**

Use a uniquely-named container on a non-default host port. Do NOT use `infra/local/` compose.

Run:
```bash
docker run -d --rm --name thittam-123-check-$$ \
  -e POSTGRES_PASSWORD=super -e POSTGRES_USER=postgres -e POSTGRES_DB=postgres \
  -p 55432:5432 postgres:16
# wait for readiness
until docker exec thittam-123-check-$$ pg_isready -U postgres -q; do sleep 1; done
```
Expected: container running, `pg_isready` returns success.

- [ ] **Step 3: Prove the create branch**

Run (superuser DSN points at the throwaway container):
```bash
DSN='postgres://postgres:super@localhost:55432/postgres?sslmode=disable'
psql "$DSN" -v ON_ERROR_STOP=1 -v app_password='s3cret-create' \
  -f scripts/prod-provision-app-role.sql
psql "$DSN" -tAc "SELECT rolname, rolcanlogin FROM pg_roles WHERE rolname='thittam_app';"
```
Expected: script runs clean; the SELECT prints `thittam_app|t` (role exists, LOGIN).

- [ ] **Step 4: Prove idempotency (the ALTER branch)**

Run:
```bash
psql "$DSN" -v ON_ERROR_STOP=1 -v app_password='s3cret-rotated' \
  -f scripts/prod-provision-app-role.sql
echo "second run exit: $?"
psql "$DSN" -tAc "SELECT count(*) FROM pg_roles WHERE rolname='thittam_app';"
```
Expected: second run exits 0 (took the ALTER branch, no "role already exists" error); count is `1` (not duplicated).

- [ ] **Step 5: Prove the missing-variable guard**

Run WITHOUT `-v app_password` against a DB where the role is absent. Use a second throwaway or drop the role first:
```bash
psql "$DSN" -c "DROP ROLE IF EXISTS thittam_app;"
psql "$DSN" -v ON_ERROR_STOP=1 -f scripts/prod-provision-app-role.sql
psql "$DSN" -tAc "SELECT count(*) FROM pg_roles WHERE rolname='thittam_app';"
```
Expected: the `\warn` guard message prints and the script stops via `\quit` BEFORE any CREATE — the final count is `0` (no passwordless role was created).

- [ ] **Step 6: Tear down the throwaway container**

Run:
```bash
docker rm -f thittam-123-check-$$
```
Expected: container removed. (If `docker` is unavailable in the sandbox, SKIP steps 2-6, note in the report that the SQL was lint-read only, and flag the container check as the reviewer's/operator's responsibility — do NOT substitute `infra/local/` compose.)

- [ ] **Step 7: Confirm no Go/build impact + commit**

Run:
```bash
go build ./...   # trivially green — no Go touched
git add scripts/prod-provision-app-role.sql
git commit -m "chore(db): add secret-safe prod thittam_app provisioning SQL (#123)

Idempotent CREATE-or-ALTER of the prod thittam_app runtime role with the
password passed via a psql -v variable (never committed). Grants stay in
scripts/db-grant-app-role.sql; this only bootstraps the role. Fills the gap
that CREATE ROLE previously existed only in dev/CI with baked passwords.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Cutover runbook `docs/operations/app-role-prod-cutover.md`

**Files:**
- Create: `docs/operations/app-role-prod-cutover.md` (new `docs/operations/` directory)

**Interfaces:**
- Consumes: the Task 1 invocation contract
  (`psql "$PROD_SUPERUSER_DSN" -v ON_ERROR_STOP=1 -v app_password="…" -f scripts/prod-provision-app-role.sql`).
- Produces: nothing consumed by later tasks (final task).

**Context:** This repo has no `docs/operations/` yet — create it. The runbook sequences the manual prod cutover. The load-bearing invariant (from `infra/k8s/config/configmap.yaml`): the service manifests already reference `secretKeyRef.key: runtime_url`, so the `runtime_url` Secret key MUST exist before those manifests are applied, or pods go un-schedulable. Therefore the runbook does role+secret BEFORE any rollout. DSN per step differs: **superuser** DSN for `CREATE ROLE`, **owner** (`thittam`) DSN for grants, **runtime** (`thittam_app`) DSN for the verification. The 10 services + `retention-sweeper` consume `runtime_url`; `purge-worker` stays on `url` (owner — it needs `DROP SCHEMA`, which `thittam_app` cannot do).

- [ ] **Step 1: Write the runbook**

Create `docs/operations/app-role-prod-cutover.md` with this content:

````markdown
# Runbook — thittam_app prod cutover (#123)

Completes epic **#120** (least-privilege runtime role; audit_log append-only) in production.
Prerequisites already merged: the least-privilege GRANT/REVOKE script (`scripts/db-grant-app-role.sql`),
the provisioning script (`scripts/prod-provision-app-role.sql`, #123), and the manifest repoint (#122 —
all 10 service Deployments + `retention-sweeper` read `DATABASE_URL` from Secret `thittam-db` key
`runtime_url`; `purge-worker` stays on key `url`).

**Append-only enforcement (`REVOKE UPDATE, DELETE ON audit_log FROM thittam_app`) only takes effect in
prod once this runbook completes.** Until then services still connect as the owner and the REVOKE is a
no-op (an owner bypasses REVOKE).

Run the steps in order. You need: a prod **superuser** DSN (for `CREATE ROLE`), the prod **owner**
(`thittam`) DSN (for grants), and cluster admin for the `thittam-db` Secret.

## 0. Preflight (safety)

The service manifests already point `DATABASE_URL` at `secretKeyRef.key: runtime_url`. If those
manifests are applied to prod BEFORE the Secret has a `runtime_url` key, the pods become
un-schedulable. Confirm the current state before proceeding:

```bash
# What key do the running Deployments reference? (expect: runtime_url for services)
kubectl -n thittam get deploy iam -o jsonpath=\
'{.spec.template.spec.containers[0].env[?(@.name=="DATABASE_URL")].valueFrom.secretKeyRef.key}{"\n"}'

# Does the live Secret already have both keys? (expect: url present; runtime_url MISSING pre-cutover)
kubectl -n thittam get secret thittam-db -o jsonpath='{.data.url}{"\n"}' | head -c 8; echo
kubectl -n thittam get secret thittam-db -o jsonpath='{.data.runtime_url}{"\n"}'
```

If a Deployment already references `runtime_url` AND the Secret lacks that key, its pods are already
failing — proceed with steps 1-4 promptly to add the key, then verify. If the Deployments still
reference `url`, you will repoint them in step 5 only after the key exists.

## 1. Provision the role (SUPERUSER DSN)

Read the new password from your secret manager into a shell variable — do NOT echo it, do NOT paste it
on the command line as a literal:

```bash
read -rs APP_PW              # paste from secret manager; not echoed
psql "$PROD_SUPERUSER_DSN" -v ON_ERROR_STOP=1 \
  -v app_password="$APP_PW" \
  -f scripts/prod-provision-app-role.sql
unset APP_PW
```

Idempotent: safe to re-run (it resets the password on the second run). Verifies the role exists:

```bash
psql "$PROD_SUPERUSER_DSN" -tAc \
  "SELECT rolname, rolcanlogin FROM pg_roles WHERE rolname='thittam_app';"
# expect: thittam_app|t
```

## 2. Grant least privilege + append-only REVOKE (OWNER DSN)

Run as the owner (`thittam`), after all migrations have been applied (the GRANT needs every table to
exist):

```bash
make db-grant-app-role DB_URL="$PROD_OWNER_DSN"
```

This applies the CRUD grants, `ALTER DEFAULT PRIVILEGES`, and `REVOKE UPDATE, DELETE ON audit_log FROM
thittam_app` from `scripts/db-grant-app-role.sql`.

## 3. Add the `runtime_url` key to the `thittam-db` Secret

The runtime DSN uses the same password from step 1:
`postgres://thittam_app:<APP_PW>@postgres.thittam.svc.cluster.local:5432/thittam`

Merge the new key WITHOUT dropping the existing `url` key (re-declare both from the current values):

```bash
read -rs APP_PW              # same password as step 1
OWNER_URL=$(kubectl -n thittam get secret thittam-db -o jsonpath='{.data.url}' | base64 -d)
RUNTIME_URL="postgres://thittam_app:${APP_PW}@postgres.thittam.svc.cluster.local:5432/thittam"
kubectl -n thittam create secret generic thittam-db \
  --from-literal=url="$OWNER_URL" \
  --from-literal=runtime_url="$RUNTIME_URL" \
  --dry-run=client -o yaml | kubectl apply -f -
unset APP_PW RUNTIME_URL OWNER_URL
```

Confirm both keys now exist:

```bash
kubectl -n thittam get secret thittam-db -o jsonpath='{.data.url}{"\n"}{.data.runtime_url}{"\n"}' \
  | sed 's/./&/' >/dev/null && echo "both keys set" || echo "MISSING A KEY"
```

## 4. Roll the workloads

Services and the retention-sweeper must restart to pick up `runtime_url`:

```bash
for d in iam project-management budget-planning expense-tracking general-ledger \
         inventory-management reporting-analytics notifications document billing; do
  kubectl -n thittam rollout restart deploy "$d"
done
kubectl -n thittam rollout restart deploy retention-sweeper 2>/dev/null || true
```

Notes:
- If any Deployment still referenced `key: url` at preflight, apply the repointed manifests
  (`kubectl apply -f infra/k8s/services/`) now — the key exists as of step 3, so pods schedule.
- The two CronJobs (`retention-sweeper` if run as a CronJob, `purge-worker`) pick up the Secret on their
  next scheduled run — no rollout needed.
- **`purge-worker` intentionally stays on `key: url`** (owner) — it needs `DROP SCHEMA`, which
  `thittam_app` cannot do. Do not repoint it.

## 5. Verify append-only enforcement (RUNTIME DSN)

Connect AS `thittam_app` and prove the REVOKE bites:

```bash
RUNTIME_DSN="postgres://thittam_app:<APP_PW>@<prod-host>:5432/thittam?sslmode=require"
# INSERT must succeed:
psql "$RUNTIME_DSN" -c "INSERT INTO audit_log (id, action) VALUES (gen_random_uuid(), 'cutover-check');"
# UPDATE and DELETE must BOTH fail with: permission denied for table audit_log
psql "$RUNTIME_DSN" -c "UPDATE audit_log SET action='x' WHERE action='cutover-check';"  # expect denied
psql "$RUNTIME_DSN" -c "DELETE FROM audit_log WHERE action='cutover-check';"            # expect denied
```

Adjust the INSERT column list to `audit_log`'s actual NOT NULL columns
(`migrations/audit/001_create_audit_log.up.sql`). A successful INSERT plus two "permission denied"
errors is the proof. Also confirm a normal service request path works end-to-end (e.g. a login →
dashboard smoke test) so the runtime role's grants are sufficient.

## 6. Rollback

If `thittam_app` cannot connect or a service breaks and you need service restored NOW:

```bash
# Emergency: repoint the affected Deployment(s) back to the owner key and restart.
kubectl -n thittam patch deploy <name> --type=json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/env/<i>/valueFrom/secretKeyRef/key","value":"url"}]'
kubectl -n thittam rollout restart deploy <name>
```

Then diagnose. Steps 1-2 (role + grants) are idempotent and safe to re-run. Once fixed, repoint back to
`runtime_url` and restart. Note: while on `url`, the audit_log REVOKE is NOT enforced (owner bypasses it)
— treat rollback as temporary.

## Done

When steps 1-5 pass in prod, mark **#123** complete and close **epic #120** — least-privilege runtime
role + audit_log append-only is now enforced in production.
````

- [ ] **Step 2: Sanity-check the runbook renders + links resolve**

Run:
```bash
test -f scripts/prod-provision-app-role.sql && echo "script referenced by runbook exists"
test -f scripts/db-grant-app-role.sql && echo "grant script exists"
grep -n "runtime_url\|purge-worker\|db-grant-app-role" docs/operations/app-role-prod-cutover.md
```
Expected: both referenced scripts exist; the grep shows the key invariants are present in the runbook.

- [ ] **Step 3: Confirm no Go/build impact + commit**

Run:
```bash
go build ./...   # trivially green
git add docs/operations/app-role-prod-cutover.md
git commit -m "docs(ops): add thittam_app prod cutover runbook (#123)

Ordered cutover: preflight (runtime_url-before-apply invariant) -> provision
role (superuser DSN) -> grant + audit_log REVOKE (owner DSN) -> add runtime_url
to the thittam-db Secret -> roll services (purge-worker stays on owner url) ->
verify append-only (INSERT ok, UPDATE/DELETE denied) -> rollback. Completes
epic #120 in prod.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

- **Spec coverage:** Artifact A (`scripts/prod-provision-app-role.sql`) → Task 1; Artifact B
  (`docs/operations/app-role-prod-cutover.md`) → Task 2. Spec's testing section (throwaway-container
  validation of create/idempotent/guard branches) → Task 1 Steps 2-6. Spec's DSN-per-step, the
  `runtime_url`-before-apply invariant, purge-worker-stays-on-url, and the rollback/verify sections →
  Task 2 Step 1. All spec sections covered.
- **Placeholder scan:** the runbook uses `<APP_PW>` / `<prod-host>` / `<name>` / `<i>` as operator-filled
  placeholders in a runbook (intentional, not plan placeholders); the SQL and all test commands are
  literal and complete. No TBD/TODO.
- **Type consistency:** the invocation contract
  `psql … -v app_password="…" -f scripts/prod-provision-app-role.sql` is identical in Task 1's Interfaces,
  Task 1 Step 1 header, and Task 2's runbook. Secret name `thittam-db`, keys `url`/`runtime_url`, and the
  service list match the manifests and the spec throughout.
