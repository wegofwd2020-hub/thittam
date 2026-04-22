---
description: Create a new Architecture Decision Record in thittam_docs with auto-incremented number
---

Create a new ADR in the sibling docs repo (`../thittam_docs/docs/developers/adr/`).

ADR title: **$ARGUMENTS**

## Steps

1. List existing ADRs: `ls ../thittam_docs/docs/developers/adr/ | sort` to find the highest existing `ADR-NNN` number. Use `highest + 1` — do not fill gaps in the numbering (gaps are intentional, usually withdrawn proposals).
2. Generate a slug from the title: lowercase, hyphen-separated, 3-6 words. Strip "the", "a", articles.
3. Create `../thittam_docs/docs/developers/adr/ADR-NNN-<slug>.md` with this template:

   ```markdown
   # ADR-NNN — <Title>

   **Status:** Proposed
   **Date:** <today's date, YYYY-MM-DD>
   **Authors:** WeGoFwd2020 platform team
   **Supersedes:** —
   **Superseded by:** —
   **Related:** <link to adjacent ADRs and Rule #N if a coding rule constrains this>

   ---

   ## Context

   <The forces at play — business AND technical. What is the problem we're solving, and what constraints bind the answer?>

   <!-- If a diagram clarifies the context, use Mermaid (default per Rule #17 / ADR-017).
        System-level diagrams should follow C4 structure (context → container → component).
        Example: -->

   ```mermaid
   graph LR
       A[Client] -->|gRPC| B[Service]
       B -->|publish| C[(NATS)]
   ```

   ## Decision

   <The choice, stated as a single declarative sentence, followed by the rationale.>

   ## Consequences

   **Positive:**
   - <what improves>

   **Negative:**
   - <what costs us — be honest>

   **Neutral:**
   - <what changes but isn't strictly better or worse>

   ## Alternatives Considered

   ### Alternative A — <name>

   <One paragraph on what it was and why it was rejected. Be specific — error messages, perf numbers, compliance conflicts.>

   ### Alternative B — <name>

   <Same structure.>

   ---

   ## Implementation Notes

   <Optional — links to tracking issues, migration plans, rollout order.>
   ```

4. **Do not** auto-generate an index entry — there is no ADR index file yet. If one exists in a future run (check for `index.md` or `README.md` in `docs/developers/adr/`), update it.

5. Show me the draft before I commit. The draft should be substantive — if you don't know the context or alternatives, ask me rather than hand-waving.

## Output

- Path of the new file
- The full rendered draft
- A prompt for the Context / Decision / Alternatives fields if you need my input
