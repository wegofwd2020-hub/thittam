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
