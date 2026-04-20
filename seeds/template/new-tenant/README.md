# Seed Template — New Tenant

Use this directory as the starting point when you need to scaffold a new
tenant via seed SQL. It is intentionally minimal — one tenant, one admin
user, one vertical binding — so that you copy it, fill in the blanks,
and nothing more.

> **Not for production tenants.** Seed SQL bypasses the `CreateTenant`
> RPC's validation (plan enum, country code, role seeding logic) and
> duplicates behaviour the service already performs. For real tenants
> follow `thittam_docs/docs/operations/tenant-onboarding.md` §2 (gRPC
> path). Use this template for development, reviewer environments, and
> demo tenants only.

---

## How to use

```bash
# 1. Copy the template into a new seed directory named after the tenant.
cp -r seeds/template/new-tenant seeds/demo/<your-tenant-slug>

# 2. Fill in the placeholders — every <ANGLE_BRACKET_TOKEN> below must be
#    replaced. Leaving any in place will cause the SQL to error out.
$EDITOR seeds/demo/<your-tenant-slug>/*.sql

# 3. Run the migration set against the new tenant schema. The tenant UUID
#    you wrote into 001_tenant.sql is what this command needs.
make migrate-tenant TENANT_ID=<uuid-you-chose>

# 4. Load the seed files in numeric order.
for f in seeds/demo/<your-tenant-slug>/*.sql; do
  psql "${DB_URL}" -f "$f"
done
```

Then verify with:

```bash
psql "${DB_URL}" -c "SELECT id, slug, status FROM tenants WHERE slug = '<your-slug>';"
psql "${DB_URL}" -c "SELECT vertical_id FROM tenant_verticals WHERE tenant_id = '<uuid-you-chose>';"
```

---

## What each file does

| File                 | Purpose                                                                                                                   |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `001_tenant.sql`     | `tenants` row, `tenant_verticals` binding, and the 7 system roles (`super_admin`, `manager`, `coordinator`, `accountant`, `member`, `inventory_manager`, `project_supervisor`). Idempotent. |
| `002_admin_user.sql` | First admin user, and a `user_roles` grant of `super_admin` (role UUID resolved by name at apply time).                    |

That's it. Productions, budgets, expenses, inventory, and other domain
data are tenant-generated via the app — not scaffolded here. If you need
demo fixtures (for a review environment or a sales demo), copy a
representative seed directory instead — e.g.
`seeds/demo/xyz-construction/` — and adjust.

---

## Placeholder checklist

The SQL files use `<ANGLE_BRACKET_TOKENS>` for every field you must fill
in. Here is the full list with guidance:

### Tenant identity

- `<TENANT_UUID>` — generate one with `uuidgen` (or `SELECT gen_random_uuid()`).
  Reuse the same UUID everywhere the files reference `<TENANT_UUID>`.
- `<TENANT_NAME>` — legal company name, e.g. `Acme Construction LLC`.
- `<TENANT_SLUG>` — lowercase, hyphen-separated. The service normally
  derives this; pick something the URL-friendly form of `<TENANT_NAME>`.
- `<TENANT_PLAN>` — `starter` · `professional` · `enterprise`.

### Address (required for billing / currency derivation per #61)

- `<ADDRESS_LINE1>` — street, e.g. `123 Main Street`.
- `<ADDRESS_LINE2>` — optional; replace with `NULL` if none.
- `<CITY>` — e.g. `Milford`.
- `<COUNTRY_CODE>` — ISO 3166-1 alpha-2: `US`, `IN`, `CA`, `GB`, …
- `<POSTAL_CODE>` — ZIP / postcode.
- `<PRIMARY_CURRENCY>` — ISO 4217: `USD`, `INR`, `CAD`, `GBP`, `EUR`.

### Vertical

- `<VERTICAL_ID>` — one of the active verticals under
  `pkg/vertical/configs/`. Today: `movie-production`, `construction`,
  `events-management`, `software-development`.

### First admin user

- `<ADMIN_USER_UUID>` — another fresh UUID for the user row.
- `<ADMIN_EMAIL>` — login email.
- `<ADMIN_DISPLAY_NAME>` — human name.
- `<BCRYPT_PASSWORD_HASH>` — **do not paste plaintext passwords**.
  Generate a bcrypt hash once:
  ```bash
  htpasswd -bnBC 12 '' '<plaintext>' | tr -d ':\n'
  ```
  or in Go:
  ```go
  hash, _ := bcrypt.GenerateFromPassword([]byte("<plaintext>"), 12)
  fmt.Println(string(hash))
  ```
  Use cost 12 to match what the IAM service uses for regular accounts.

---

## After loading

The tenant has:

1. A row in `tenants`.
2. A `tenant_verticals` binding so vertical-aware services know which
   entity labels and phase types to use.
3. Seven system roles (`super_admin`, `manager`, `coordinator`,
   `accountant`, `member`, `inventory_manager`, `project_supervisor`)
   seeded by `001_tenant.sql` — mirrors `iam.Service.seedSystemRoles`
   which the `CreateTenant` gRPC would otherwise do.
4. One admin user with the `super_admin` role (tenant-wide;
   `user_roles.project_id = NULL`).

No projects, budgets, expenses, or inventory — the admin user creates
those from the app.

---

## Worked example

The existing `seeds/demo/xyz-construction/` directory (merged in #67) is
a complete worked example built from this template. Skim it if the
placeholders here are unclear — everything in `001_tenant.sql` and
`002_users.sql` of that seed maps back to a `<PLACEHOLDER>` in this
template.
