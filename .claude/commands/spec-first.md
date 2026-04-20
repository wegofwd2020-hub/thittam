---
description: Force a spec (test + contract + acceptance) before any implementation
---

Task: **$ARGUMENTS**

Before writing any implementation code, produce the spec below and wait for my explicit approval. Do not edit non-test files until I say "approved".

## 1. Failing test(s)

Write the Go test files that will pass once the feature is implemented. Use:
- `t.Parallel()` on every test
- Hand-written mocks (function-field pattern), not `testify/mock`
- Deterministic UUIDs (`uuid.MustParse(...)`)
- Fixed timestamps, never `time.Now()` in assertions
- `testify/assert` + `testify/require`

For vertical-aware services, include a test with `vertical.WithConfig(ctx, fixture)`.

## 2. API contract

- **gRPC method signature** (proto snippet) if this is a cross-service call
- **Request / response schema** with every field typed — monetary fields MUST be `decimal.Decimal` / `NUMERIC(14,2)` / JSON string (Rule #1)
- **Error matrix** — list every gRPC status code + sentinel error (`ErrNotFound`, `ErrConflict`, etc.) the handler can return, with the trigger condition

## 3. Acceptance checklist

Confirm each item is either addressed by the design or explicitly N/A:

- [ ] **Idempotency** — how repeat calls are deduplicated (Rule #5). `Idempotency-Key` header on POST, `ON CONFLICT` on writes, or event `event_id` dedup.
- [ ] **Vertical config** — if the service is vertical-aware, name which YAML fields under `pkg/vertical/configs/` are read.
- [ ] **Audit log** — for auth events, financial ops, admin actions, or impersonation, specify the `action`, `target_type`, `old_state`, `new_state` (Rule #7).
- [ ] **Observability** — structured log fields (`service`, `method`, `tenant_id`, `request_id`), metric name if new, trace span name.
- [ ] **RBAC scope** — which roles pass; which tenant isolation check fires (schema selector + any RLS).
- [ ] **Secret tier** — T1/T2/T3/T4, source (Vault vs env), startup health check impact.
- [ ] **Cache plan** — L1 / L2 / L3 strategy, TTL, invalidation trigger (Rule #3).

## 4. Stop and wait

End your response with: *"Spec ready — approve or request changes."*

Do not proceed to implementation until I reply "approved".
