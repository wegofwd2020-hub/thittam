---
description: Add a new vertical plugin (YAML config, status enum, icon set, seed)
---

Add a new vertical plugin named **$ARGUMENTS**.

## Before creating

Read an existing vertical config end-to-end for reference:
- `pkg/vertical/configs/movie-production.yaml`
- `pkg/vertical/configs/construction.yaml`
- `pkg/vertical/configs/events-management.yaml`
- `pkg/vertical/configs/software-development.yaml`

Then confirm with me:
- The industry display name (e.g., "Catering Services")
- 3-5 distinct **production phases** typical for this vertical
- 3-5 distinct **expense categories** typical for this vertical
- A curated **icon subset** (8-12 lucide-react icons relevant to this industry, per Rule #18 per-vertical icon sets)

## Files to create

1. `pkg/vertical/configs/$ARGUMENTS.yaml` — full vertical YAML config. Must validate against `pkg/vertical/schema.json`.
2. A fixture entry under `pkg/vertical/testdata/` so loader tests cover this vertical.
3. Seed data — add a sample tenant row in the seed fixtures that uses this vertical.
4. Frontend icon mapping — add to the vertical theme config in `web/` (locate the existing mapping before editing).

## Validation

After writing the YAML:

```bash
go test ./pkg/vertical/... -run TestValidator
```

If schema validation fails, fix the YAML — do not change the schema without a separate discussion (schema changes are breaking for all existing verticals).

## Docs

Add a short section to `thittam_docs/docs/verticals/` describing the new vertical — target persona, phases, and any compliance notes.

## Output

After creating the files, show:
1. The YAML diff
2. Test output for `TestValidator`
3. A checklist of downstream updates (icon mapping, docs, seed)

Do not commit — let me review first.
