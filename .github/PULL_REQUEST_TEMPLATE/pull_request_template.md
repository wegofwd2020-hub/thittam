## Summary

<!-- What does this PR do? 1-3 bullet points. -->
- 

## Motivation

<!-- Why is this change needed? Link to the issue: Fixes #NNN -->

## Type of change

- [ ] `feat` — new feature
- [ ] `fix` — bug fix
- [ ] `refactor` — no behaviour change
- [ ] `chore` — maintenance (deps, CI, tooling)
- [ ] `docs` — documentation only
- [ ] `test` — tests only
- [ ] `security` — security fix

## Scope

<!-- Which services/packages does this touch? -->
- [ ] iam
- [ ] general-ledger
- [ ] budget-planning
- [ ] expense-tracking
- [ ] project-management
- [ ] inventory-management
- [ ] reporting-analytics
- [ ] notifications
- [ ] document
- [ ] pkg/vertical
- [ ] infra / CI
- [ ] web (frontend)

## Test plan

<!-- How was this tested? -->

- [ ] Unit tests added/updated
- [ ] `go test ./... -short` passes locally
- [ ] `make validate-verticals` passes (if vertical configs changed)
- [ ] Manual smoke test (describe below if applicable)

### Test count

| Package | Before | After | Delta |
|---|---|---|---|
| services/iam | | | |
| services/ledger | | | |
| services/budget | | | |
| services/expense | | | |
| services/document | | | |
| services/inventory | | | |
| services/reporting | | | |
| services/project | | | |
| services/notifications | | | |
| **Total** | | | |

<!-- Run: go test ./... -short | grep -E "^ok" to get counts -->

## Coverage impact

<!-- Run: go test ./... -short -coverprofile=coverage.out && go tool cover -func=coverage.out | grep total -->

| Package | Coverage | Threshold | Status |
|---|---|---|---|
| services/iam | | ≥85% | |
| services/ledger | | ≥85% | |
| services/budget | | ≥80% | |
| services/expense | | ≥80% | |
| services/document | | ≥75% | |
| services/inventory | | ≥75% | |
| services/reporting | | ≥75% | |

## Breaking changes

<!-- Does this change any gRPC/protobuf contracts, DB schema, or public API? -->
- [ ] None
- [ ] Yes — describe migration path:

## Checklist

- [ ] Conventional Commit message (`feat(scope): summary`)
- [ ] No secrets or credentials in code
- [ ] Money values use `decimal.Decimal` / `NUMERIC(14,2)` — never `float64`
- [ ] 2 reviewers requested; senior engineer included for iam/ledger/security changes
- [ ] Architecture diagrams updated if service topology changed (see Rule #17)
