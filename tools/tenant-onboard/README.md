# tenant-onboard

Generates Thittam **tenant seed SQL** from a validated `company.yaml`, and
**vertical registration migrations** from vertical YAML. Specs:
[`add-us-studio-onboarding`](../../openspec/changes/add-us-studio-onboarding/),
[`add-partnership-books`](../../openspec/changes/add-partnership-books/).

Python ≥ 3.10, `PyYAML`; `bcrypt` optional (for `hash-password`).

## Commands

Run from the repo root (`PYTHONPATH=tools/tenant-onboard python3 -m tenant_onboard …`,
or use the Make targets).

| Command | Purpose | Make target |
| --- | --- | --- |
| `validate --config C [--vertical V]` | Validate a company config | `tenant-validate TENANT=…` |
| `tenant --config C --out DIR` | Write 001–004 `.sql` | `tenant-generate`, `seed-tenant` |
| `vertical-migration --vertical V --up U --down D [--restores P]` | Render vertical migration | `vertical-migrations` |
| `check-vertical-migration …` | Fail on drift | `check-vertical-migrations` |
| `hash-password [--cost 12]` | Prompt, print bcrypt hash | — |

`--restores P` marks an *update* migration: `P` is the earlier `.up.sql` that
registered the vertical, and the down migration restores that definition
instead of deleting the vertical. Only the newest migration per vertical is
generated and drift-checked; earlier ones are frozen history (see
`VERTICAL_MIGRATIONS` in the `Makefile`).

Without `--vertical`, the vertical is looked up as
`pkg/vertical/configs/<tenant.vertical>.yaml` (`--verticals-dir` to change).

Exit codes: `0` ok · `1` check failed · `2` bad input · `3` cannot write ·
`70` internal error (`--debug` for traceback). Validation reports **all**
issues at once, each as `field.path: message`.

## company.yaml

```yaml
tenant:   {id: <uuid>, name: …, slug: kebab-case, plan: starter|professional|enterprise,
           vertical: <vertical id>, primary_currency: USD}
address:  {line1, line2 (nullable), city, postal_code (quoted), country: US}
admin:    {id: <uuid ≠ tenant.id>, email, display_name}
books:    {opening_date: YYYY-MM-DD, fiscal_year_end_month: 1-12 (default 12)}
products: [{slug, title, description?, stage: <vertical stage>, start_date?, end_date?}]
stage_status_map: {<stage>: <productions.status>}   # optional overrides
entity:                                              # optional; required by software-development-us
  tax_classification: partnership_1065 | disregarded_schedule_c
  members:                                           # mapping, so the local file can add names
    <key>: {label: Member A, name?: <real name>, ownership_pct: 50}
```

### Owner equity (`entity:`)

The vertical's chart stops at `3000 Equity`; the accounts under it depend on
who owns the business, so they are generated from `entity:` and appended to
`004_ledger.sql` after the vertical's accounts.

| Classification | Members | Generated accounts |
| --- | --- | --- |
| `disregarded_schedule_c` | exactly 1 | `3100 Owner's Contributions`, `3200 Owner's Draws` |
| `partnership_1065` | 2–9 | per member *n*: `30n0 Capital — …`, `31n0 Distributions — …`, `605n Guaranteed Payments — …`; plus `3910 Current-Year Earnings (clearing)` and `6050 Guaranteed Payments to Partners` (Form 1065 line 10) |

`ownership_pct` must total exactly 100 (decimals allowed, up to 4 places).
Account names use `name` when the local file supplies it, otherwise `label`,
so a public repo never holds members' names. `s_corp_1120s` and
`c_corp_1120` are rejected as not supported yet.

Without an `entity:` block nothing is generated and other verticals behave
exactly as before. For a tenant first seeded from `software-development-us`
v1.0.0, the ledger seed also renames the old single-owner accounts (`3000
Owner's Capital` → `Equity`, `2300 Due to Owner` → `Due to Owners`) and, for a
partnership, deactivates `3100`/`3200`. Those updates match on code *and* old
name, so they never touch an account renamed in the app, and nothing is
deleted.

`company.local.yaml` beside it is deep-merged on top (lists replace). Any value
still containing `REPLACE_ME` after the merge is an error.

## Layout

```
tenant_onboard/
  errors.py    exception hierarchy (OnboardError → ValidationError → …)
  sql.py       safe literal rendering — the only path from values to SQL text
  yamlio.py    YAML loading with precise file errors
  vertical.py  vertical model, CoA checks, migration rendering
  entity.py    entity block, owner-equity accounts, legacy single-owner fixes
  company.py   company model, override merge, validation, cross-check
  render.py    seed SQL rendering + atomic file writes
  cli.py       argparse front-end, exit-code mapping
tests/
  fixtures/    mock data only (fictitious companies, minimal vertical)
  test_*.py    unit tests; test_repo_drift.py checks lockstep with the repo
  test_integration_pg.py   real PostgreSQL (opt-in, see below)
```

## Tests

```bash
make test-tenant-onboard                     # unit + repo drift
THITTAM_TEST_DSN=postgresql://… \
  python3 -m pytest -m integration           # + real PostgreSQL
```

The integration test creates and drops its own schema in the DSN's database
and never touches `public`. Point it at a disposable instance — per
`CLAUDE.md`, never at the `infra/local` compose stack.
