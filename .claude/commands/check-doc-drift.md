---
description: Run tools/check-doc-drift against thittam_docs and report findings
---

Run the doc-drift check (Rule #16) against the sibling docs repo.

## Steps

1. Verify `../thittam_docs/` exists. If not, stop and tell me to clone it.
2. Run:

   ```bash
   go run ./tools/check-doc-drift -docs ../thittam_docs
   ```

   (If the tool takes different flags, read `tools/check-doc-drift/main.go` to discover the correct invocation — do not guess.)

3. Parse the output. For each violation:
   - Identify the referenced identifier (function / type / method)
   - Locate the doc file + line number
   - Determine whether the identifier was **renamed**, **removed**, or **never existed**
   - Suggest whether to: (a) update the doc, (b) restore the identifier, or (c) mark the code block with `<!-- drift:ignore -->` if it's intentional pseudocode

4. Do not auto-fix. Produce a ranked list:
   - **Must fix** — exported identifier referenced in docs that is now missing
   - **Should fix** — renamed identifier with docs still using the old name
   - **Consider** — code blocks that might warrant `drift:ignore`

## Output

Plain markdown table:

| Severity | Doc file:line | Identifier | Action |
|---|---|---|---|
| must | docs/api/budgets.md:42 | `SubmitBudgetRequest` | renamed to `CreateBudgetRequest` — update doc |

End with a one-line summary: `N must-fix, M should-fix, K consider`.
