---
description: Add or extend a proto file for a Thittam service, regenerate, and scaffold the handler stub
---

Add or extend the proto for service **$ARGUMENTS**.

## Before editing

1. Check whether `proto/thittam/$ARGUMENTS/` already exists.
2. If it does, confirm with me which RPC method(s) to add (name, request, response, error semantics).
3. If it does not, confirm with me the service's full RPC surface (list of methods with signatures).
4. Run `buf breaking proto --against '.git#branch=main,subdir=proto'` once the edits are drafted — fail loudly if the change is breaking; ask me whether to proceed.

## Edits

- Add / extend `proto/thittam/$ARGUMENTS/v1/service.proto`.
- Field types for money: `string` in proto (not `double`), serialised as decimal (Rule #1). Add a comment noting this.
- Field types for IDs: `string` (UUID).
- Every RPC method must have explicit error documentation: `// Errors: NOT_FOUND, INVALID_ARGUMENT, PERMISSION_DENIED`.

## Regenerate

```bash
buf lint
buf generate
```

If either fails, fix the proto — do not edit generated files.

## Scaffold handler stub

For each new RPC method, add a stub in `services/$ARGUMENTS/handler.go` that:
1. Reads `tenant_id` from context (fail `PERMISSION_DENIED` if absent)
2. Validates the request at the boundary (Rule #11)
3. Delegates to the `Service` layer
4. Maps sentinel errors to gRPC status codes, logging once at the handler (Rule #12)
5. Returns `codes.Unimplemented` initially with a `TODO` comment linking to the issue

Add a matching `handler_test.go` test that exercises the unimplemented code path.

## Output

Show:
1. The proto diff
2. `buf lint` + `buf generate` output (success or failure)
3. The handler stub diff
4. Next recommended step (usually: implement the Service method per `/spec-first`)

Do not commit.
