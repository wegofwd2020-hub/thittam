# tenant-onboard

Generates Thittam **tenant seed SQL** from a validated `company.yaml`, and
**vertical registration migrations** from vertical YAML. Spec:
[`openspec/changes/add-us-studio-onboarding`](../../openspec/changes/add-us-studio-onboarding/).

Python ≥ 3.10, `PyYAML`; `bcrypt` optional (for `hash-password`).

## Commands

Run from the repo root (`PYTHONPATH=tools/tenant-onboard python3 -m tenant_onboard …`,
or use the Make targets).

| Command | Purpose | Make target |
| --- | --- | --- |
| `validate --config C [--vertical V]` | Validate a company config | `tenant-validate TENANT=…` |
| `tenant --config C --out DIR` | Write 001–004 `.sql` | `tenant-generate`, `seed-tenant` |
| `vertical-migration --vertical V --up U --down D` | Render vertical migration | `vertical-migrations` |
| `check-vertical-migration …` | Fail on drift | `check-vertical-migrations` |
| `hash-password [--cost 12]` | Prompt, print bcrypt hash | — |

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
```

`company.local.yaml` beside it is deep-merged on top (lists replace). Any value
still containing `REPLACE_ME` after the merge is an error.

## Layout

```
tenant_onboard/
  errors.py    exception hierarchy (OnboardError → ValidationError → …)
  sql.py       safe literal rendering — the only path from values to SQL text
  yamlio.py    YAML loading with precise file errors
  vertical.py  vertical model, CoA checks, migration rendering
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
