# Real tenants

One directory per **real** (non-demo) tenant. Unlike `seeds/demo/`, nothing
here is hand-written SQL: `company.yaml` is the source of truth and
`tools/tenant-onboard` generates the SQL into `build/seeds/<slug>/`
(git-ignored).

| Slug             | Company            | Vertical                | Books open  |
| ---------------- | ------------------ | ----------------------- | ----------- |
| `kaundinya-labs` | Kaundinya Labs LLC | software-development-us | 2026-10-20  |

## Loading a tenant (local Postgres)

```bash
# 0. Once per DB: apply migrations (registers the vertical via shared/007).
make migrate-all

# 1. Private details — never committed (git-ignored).
cp seeds/tenants/kaundinya-labs/company.local.yaml.example \
   seeds/tenants/kaundinya-labs/company.local.yaml
$EDITOR seeds/tenants/kaundinya-labs/company.local.yaml

# 2. Validate — reports every problem at once.
make tenant-validate TENANT=kaundinya-labs

# 3. Admin password → bcrypt hash (prompts; needs `pip install bcrypt`).
PYTHONPATH=tools/tenant-onboard python3 -m tenant_onboard hash-password

# 4. Generate + load. Single quotes matter: the hash contains `$`.
ADMIN_PASSWORD_HASH='$2b$12$...' make seed-tenant TENANT=kaundinya-labs
```

Every file is one transaction and every insert is `ON CONFLICT DO NOTHING`, so
step 4 is safe to re-run. It will **not** change an existing user's password or
an existing product — do that in the app.

Verify:

```bash
psql "$DB_URL" -c "SELECT slug, status FROM productions
                   WHERE tenant_id = '960b876f-a3f4-4e73-9cb7-33a77870df50';"
```

Then log in as the Owner and post the opening entry (owner's contribution →
`3100`, bank → `1010`) dated 2026-10-20.

## Adding another tenant

1. `mkdir seeds/tenants/<slug>`; copy `kaundinya-labs/company.yaml` and its
   `.example`; give it fresh UUIDs (`python3 -c "import uuid; print(uuid.uuid4())"`).
2. Keep anything private as `REPLACE_ME` in `company.yaml`.
3. Add a row to the table above.

## Why not the `CreateTenant` gRPC path?

`thittam_docs/.../tenant-onboarding.md` §2 is the production path and also emits
registration events. For a single self-hosted tenant the generator gives the same
rows with validation that mirrors the DB constraints, and works before the full
service mesh is running. If you later run the full stack, the ledger's
`SeedChartOfAccounts` is idempotent against what this seed created.
