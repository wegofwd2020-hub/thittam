# Web CI Gate + Lint Cleanup (#179) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-24
**Issue:** #179 (nothing gates `web/` lint or build — main carries a lint error and 20 warnings)
**Branch:** `ci/web-lint-build-179` off `main` (`976419d`)
**Migration:** none

## Goal

Make `web/` lint and build a CI gate so they can never silently regress again,
and clear the existing 1 error + 20 warnings on `main` so the gate can sit at
`--max-warnings 0`.

## Context

`.github/workflows/ci.yml` is Go-only — it never touches `web/`.
`.github/workflows/ui-e2e.yml` triggers on `web/**` but runs only
`npx playwright test --project=smoke`; it never runs `npm run lint` or
`npm run build`. So `web/`'s lint and build are verified only when someone
happens to run them locally, and a lint **error** has sat on `main` since
April (introduced by `228a7d9`, #31).

This is the same failure mode as #160 (billing sqlc drift): no gate, so it
accumulates silently. #160 is now closed by a `Codegen Freshness (sqlc)` job
in `ci.yml` that runs on every PR. `web/` gets the equivalent here.

Current lint state on `main` (`npm run lint` in `web/`, measured 2026-07-24):

```
✖ 21 problems (1 error, 20 warnings)
```

- **1 error** — `web/src/lib/accessibility/dyslexia-provider.tsx:24`,
  `react-hooks/set-state-in-effect`: a `setState` called synchronously in a
  `useEffect` body that reads a localStorage preference on mount.
- **20 warnings** — ~13 `@typescript-eslint/no-unused-vars`, 6
  `react-hooks/exhaustive-deps` (a logical-expression `const` used in a
  `useMemo` dep list), and 1 unused `eslint-disable` directive.

`npm run build` already passes (exit 0), verified 2026-07-24. So once lint is
clean the build step of the new job is green.

Tooling facts:
- `web/eslint.config.mjs` is a flat config: `eslint-config-next/core-web-vitals`
  + `eslint-config-next/typescript`.
- `web/package.json` `lint` script is bare `eslint` (not `next lint`), so
  `npm run lint -- --max-warnings 0` passes the flag straight through to eslint.
- `web/package-lock.json` exists; `ui-e2e.yml` already pins `node-version: "20"`
  with `cache: npm`.

## Design

### 1. eslint config — honor the `^_` "intentionally unused" convention

Two of the unused-var warnings are **already** underscore-prefixed
(`_productionId` in `components/expenses/expense-form.tsx:28`, `_params` in
`lib/api/reports.ts:261`) — deliberate markers that the flat config does not yet
honor. Add one rule override to `web/eslint.config.mjs`:

```js
{
  rules: {
    "@typescript-eslint/no-unused-vars": [
      "warn",
      { argsIgnorePattern: "^_", varsIgnorePattern: "^_", caughtErrorsIgnorePattern: "^_" },
    ],
  },
},
```

Underscore now means "intentionally unused" across the codebase; genuinely-dead
vars (no underscore) still warn. This is the standard TypeScript-eslint idiom,
not a blanket relaxation. It must land **first** so the vars cleanup (§3)
targets exactly the set that remains.

### 2. The error — `dyslexia-provider.tsx` → `useSyncExternalStore`

Replace the `useState(false)` + mount-effect-`setState` pattern with
`useSyncExternalStore`, the React-blessed primitive for reading browser/external
state:

- `getServerSnapshot: () => false` — SSR renders the default, no mismatch.
- `getSnapshot: () => localStorage.getItem(STORAGE_KEY) === "true"` — client reads
  the real preference.
- `subscribe(cb)` — registers `cb` for a change signal so `toggleDyslexia` (and
  cross-tab `storage` events) re-render consumers.
- A separate `useEffect` keyed on the resolved `dyslexiaMode` calls
  `applyDyslexiaStyles(dyslexiaMode)` — it only mutates `document`, never
  `setState`, so `set-state-in-effect` does not fire.

`toggleDyslexia` writes localStorage then notifies subscribers (instead of
calling a React setter). This removes the second-render-before-paint flash of
un-styled content on load, not just the lint error. Public API of the module —
`AccessibilityProvider`, `useAccessibility`, the context shape
`{ dyslexiaMode, toggleDyslexia }` — is unchanged, so no consumer changes.

### 3. Unused-vars cleanup (~13 + 1 directive)

Delete genuinely-dead imports and locals:

| file:line | symbol |
|---|---|
| `lib/fonts.ts:13` | `localFont` (dead import) |
| `app/(dashboard)/page.tsx:12,13` | `StatusBadge`, `AmountDisplay` (dead imports) |
| `app/(dashboard)/inventory/[id]/page.tsx:344` | `isRetired` |
| `app/(dashboard)/inventory/new/page.tsx:61` | `entityLabels` |
| `app/(dashboard)/settings/page.tsx:377,681` | `entityLabels`, `values` |
| `app/(dashboard)/team/page.tsx:175,282` | `router`, `row` |
| `components/settings/role-permission-matrix.tsx:29` | `resource` |
| `components/settings/theme-customizer.tsx:110` | `resetColor` |
| `app/(platform)/verticals/page.tsx:152` | remove the unused `eslint-disable` directive line |

The two API-stub params (`_productionId`, `_params`) keep their names and stay
underscored — now silenced by §1, no edit needed. Each deletion is
remove-the-declaration only; where a symbol is a destructure or callback param
whose siblings are still used, drop just that name from the pattern. Any
follow-on "now this import is fully unused" is removed in the same pass.

### 4. exhaustive-deps cleanup (6)

Three files compute `const x = something ?? []` (or a `&&`/`||` expression) and
then use `x` in a `useMemo` dependency array; a fresh array/object identity each
render defeats the memo. Fix each by wrapping the initialization in its own
`useMemo` so the dependency is identity-stable:

```tsx
// before
const lineItems = data?.items ?? [];
const total = useMemo(() => sum(lineItems), [lineItems]);   // warns

// after
const lineItems = useMemo(() => data?.items ?? [], [data]);
const total = useMemo(() => sum(lineItems), [lineItems]);   // clean
```

Sites (from the lint run, measured 2026-07-24):

| file | line | var | # useMemo deps affected |
|---|---|---|---|
| `app/(dashboard)/budgets/[id]/page.tsx` | 79 | `lineItems` | 3 |
| `app/(dashboard)/budgets/page.tsx` | 45 | `productions` | 2 |
| `app/(dashboard)/productions/page.tsx` | 81 | `productions` | 1 |

Each is a real, if minor, perf fix — not a suppression. No `eslint-disable`
is added.

### 5. CI job `web-lint-build` in `ci.yml`

Add one job to `.github/workflows/ci.yml`, **no path filter** — it runs on every
push/PR exactly like the Go jobs and `Codegen Freshness`, so it is an
unskippable required check that cannot be dodged by a PR that "doesn't look like"
a web change:

```yaml
web-lint-build:
  name: Web Lint & Build
  runs-on: ubuntu-latest
  defaults:
    run:
      working-directory: web
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-node@v4
      with:
        node-version: "20"
        cache: npm
        cache-dependency-path: web/package-lock.json
    - name: Install dependencies
      run: npm ci
    - name: Lint (zero warnings allowed)
      run: npm run lint -- --max-warnings 0
    - name: Build
      run: npm run build
```

`node-version: "20"` matches the existing `ui-e2e.yml` (one web toolchain
version across CI; bumping both to 22 is a separate concern, out of scope).
`npm ci` is npm-cached on `web/package-lock.json`, so the steady-state cost on a
Go-only PR is ~1 min. Placed after the existing jobs in `ci.yml`.

## What this does and does not do

- **Does:** gate lint (`--max-warnings 0`) and build for `web/` on every PR; fix
  the one error and every current warning; honor `^_` for intentional unused.
- **Does not:** touch `ui-e2e.yml` / Playwright, change the Node version of the
  existing web CI, run the authenticated Playwright suite, add any product
  behavior, or restructure the flat eslint config beyond the one rule override.

## Testing

- **The gate itself is the test:** after §1–§4, `npm run lint -- --max-warnings 0`
  exits 0 and `npm run build` exits 0 locally; the new job proves it in CI.
- **Regression proof for the error fix:** the `set-state-in-effect` rule reports
  zero for `dyslexia-provider.tsx`, and the component still (a) reads the stored
  preference on load and (b) toggles + persists it. No unit test harness exists
  for `web/` beyond Playwright; correctness of the provider is verified by lint
  passing plus a manual note that the public API is unchanged.
- **Prove the gate has teeth:** the CI step must fail if a warning is
  reintroduced — confirmed by the `--max-warnings 0` flag (a single warning
  makes eslint exit non-zero). The final whole-branch review checks that the job
  is not accidentally path-filtered or `continue-on-error`.

## Non-goals

- No change to `ui-e2e.yml` or the Playwright projects.
- No Node 22 bump.
- No `eslint-disable` suppressions to reach zero — every finding is fixed at the
  source.
- No new lint rules beyond the `no-unused-vars` `^_` override.
- No product/UI behavior change; `dyslexia-provider`'s public API is preserved.

## Review weight

Touches `web/` (frontend) and `.github/workflows/ci.yml` (CI). Not `iam` /
`general-ledger` / security, so standard 2 approvals — no mandatory senior
engineer. Whole-branch review at the end per the usual workflow.
