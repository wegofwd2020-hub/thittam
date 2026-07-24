# Web CI Lint+Build Gate (#179) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `web/` lint (`--max-warnings 0`) and build a CI gate, and clear the 1 lint error + 20 warnings currently on `main` so the gate passes.

**Architecture:** Fix the source findings in four passes (eslint config first, then the error, then unused-vars, then exhaustive-deps), then add an always-run `web-lint-build` job to `.github/workflows/ci.yml` mirroring the existing `Codegen Freshness (sqlc)` gate. There is no unit-test harness for `web/` beyond Playwright, so **`npm run lint` is the test** for each task: confirm the specific finding exists, fix it, confirm it is gone.

**Tech Stack:** Next.js (App Router) + React 19 + TypeScript, ESLint flat config (`eslint-config-next` core-web-vitals + typescript), GitHub Actions, Node 20.

## Global Constraints

- **Node version `"20"`** in the CI job — matches the existing `.github/workflows/ui-e2e.yml`; do NOT bump to 22 (out of scope).
- **Gate threshold is `--max-warnings 0`** — the end state is zero errors and zero warnings.
- **No `eslint-disable` suppressions** — every finding is fixed at the source. The only rule-config change is the `^_` override in Task 1.
- **`^_` means "intentionally unused"** after Task 1: dead imports/top-level consts are DELETED; a dead function/callback parameter that must keep its position is renamed to `_name`.
- **`dyslexia-provider.tsx`'s public API is unchanged** — `AccessibilityProvider`, `useAccessibility`, and the context shape `{ dyslexiaMode: boolean; toggleDyslexia: () => void }` stay identical.
- **Do NOT touch** `.github/workflows/ui-e2e.yml`, the Playwright projects, or any product/UI behavior.
- **Commits:** Conventional Commits, scope `web` or `ci`; end every commit message with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- All `npm` commands run **from the `web/` directory**.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `web/eslint.config.mjs` | ESLint flat config — add `no-unused-vars` `^_` override | 1 |
| `web/src/lib/accessibility/dyslexia-provider.tsx` | Fix `set-state-in-effect` error via `useSyncExternalStore` | 2 |
| ~11 `web/src/**` files | Remove dead imports/vars + 1 dead `eslint-disable` | 3 |
| 3 `web/src/app/(dashboard)/**` pages | Wrap 3 `const … ?? []` initializers in `useMemo` | 4 |
| `.github/workflows/ci.yml` | New always-run `web-lint-build` job | 5 |

---

### Task 1: ESLint config — honor the `^_` intentional-unused convention

**Files:**
- Modify: `web/eslint.config.mjs`

**Interfaces:**
- Consumes: nothing.
- Produces: after this task, ESLint's `@typescript-eslint/no-unused-vars` ignores any identifier matching `^_`. Later tasks rely on this so a dead callback param can be silenced by renaming to `_name` rather than deleted.

- [ ] **Step 1: Confirm the two underscore-prefixed findings currently warn**

Run (from `web/`):
```bash
npm run lint 2>&1 | grep -E "_productionId|_params"
```
Expected: two `warning … '_productionId' is defined but never used` / `'_params' is defined but never used` lines. (This proves the config does NOT yet honor `^_`.)

- [ ] **Step 2: Add the rule override**

The current file is:
```js
import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
]);

export default eslintConfig;
```

Add a config object with the rule override **after** the `...nextTs` spread (so it wins) and before `globalIgnores`:
```js
const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Treat a leading underscore as "intentionally unused" — the standard
  // TypeScript-eslint idiom. Genuinely-dead identifiers (no underscore) still
  // warn; deliberate placeholders like `_params` on a stub signature do not.
  {
    rules: {
      "@typescript-eslint/no-unused-vars": [
        "warn",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
        },
      ],
    },
  },
  // Override default ignores of eslint-config-next.
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
]);
```

- [ ] **Step 3: Confirm the two underscore findings are gone and lint still runs**

Run (from `web/`):
```bash
npm run lint 2>&1 | grep -E "_productionId|_params" || echo "CLEARED"
npm run lint 2>&1 | tail -1
```
Expected: first command prints `CLEARED`; last line still shows the remaining tally (`✖ 19 problems (1 error, 18 warnings)` — the two underscore warnings dropped from 20 → 18). A non-zero problem count here is expected; later tasks clear the rest.

- [ ] **Step 4: Commit**

```bash
git add web/eslint.config.mjs
git commit -m "chore(web): honor ^_ prefix in no-unused-vars

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Fix the `set-state-in-effect` error in `dyslexia-provider.tsx`

**Files:**
- Modify: `web/src/lib/accessibility/dyslexia-provider.tsx`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: unchanged public API — `AccessibilityProvider`, `useAccessibility`, context `{ dyslexiaMode, toggleDyslexia }`. No consumer edits.

- [ ] **Step 1: Confirm the error currently fires**

Run (from `web/`):
```bash
npm run lint 2>&1 | grep "set-state-in-effect"
```
Expected: one `error … react-hooks/set-state-in-effect` line for `dyslexia-provider.tsx`.

- [ ] **Step 2: Replace the state+effect pattern with `useSyncExternalStore`**

Replace the entire file with:
```tsx
"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useSyncExternalStore,
} from "react";

interface AccessibilityState {
  dyslexiaMode: boolean;
  toggleDyslexia: () => void;
}

const AccessibilityContext = createContext<AccessibilityState>({
  dyslexiaMode: false,
  toggleDyslexia: () => {},
});

const STORAGE_KEY = "thittam-dyslexia-mode";

// The dyslexia preference lives in localStorage, not React state.
// useSyncExternalStore reads it without a mount-effect setState — that pattern
// triggers a second render before paint (a visible flash of un-styled content)
// and trips react-hooks/set-state-in-effect. getServerSnapshot returns the
// default so SSR and the first client render agree (no hydration mismatch).

const listeners = new Set<() => void>();

function emitChange() {
  for (const listener of listeners) listener();
}

function subscribe(callback: () => void) {
  listeners.add(callback);
  // Reflect changes made in other tabs, too.
  window.addEventListener("storage", callback);
  return () => {
    listeners.delete(callback);
    window.removeEventListener("storage", callback);
  };
}

function getSnapshot() {
  return localStorage.getItem(STORAGE_KEY) === "true";
}

function getServerSnapshot() {
  return false;
}

export function AccessibilityProvider({ children }: { children: React.ReactNode }) {
  const dyslexiaMode = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  // Apply/remove the document-level styles whenever the resolved preference
  // changes. This effect only mutates the DOM; it never calls setState, so
  // react-hooks/set-state-in-effect does not fire.
  useEffect(() => {
    applyDyslexiaStyles(dyslexiaMode);
  }, [dyslexiaMode]);

  const toggleDyslexia = useCallback(() => {
    const next = localStorage.getItem(STORAGE_KEY) !== "true";
    localStorage.setItem(STORAGE_KEY, String(next));
    emitChange();
  }, []);

  return (
    <AccessibilityContext.Provider value={{ dyslexiaMode, toggleDyslexia }}>
      {children}
    </AccessibilityContext.Provider>
  );
}

export function useAccessibility() {
  return useContext(AccessibilityContext);
}

// Applies/removes dyslexia-friendly styles on the document root.
function applyDyslexiaStyles(enabled: boolean) {
  const root = document.documentElement;

  if (enabled) {
    root.classList.add("dyslexia-mode");
    root.style.setProperty("--font-heading", "'OpenDyslexic', sans-serif");
    root.style.setProperty("--font-body", "'OpenDyslexic', serif");
    root.style.setProperty("--font-mono", "'OpenDyslexic Mono', monospace");
    root.style.setProperty("--letter-spacing", "0.05em");
    root.style.setProperty("--line-height", "1.8");
  } else {
    root.classList.remove("dyslexia-mode");
    root.style.removeProperty("--font-heading");
    root.style.removeProperty("--font-body");
    root.style.removeProperty("--font-mono");
    root.style.removeProperty("--letter-spacing");
    root.style.removeProperty("--line-height");
  }
}
```

Key points: `subscribe`/`getSnapshot` are only invoked client-side (React calls `getServerSnapshot` during SSR), so `window`/`localStorage` are safe unguarded. `toggleDyslexia` writes storage then `emitChange()` — no React setter.

- [ ] **Step 3: Confirm the error is gone**

Run (from `web/`):
```bash
npm run lint 2>&1 | grep "set-state-in-effect" || echo "CLEARED"
npm run lint 2>&1 | tail -1
```
Expected: first prints `CLEARED`; tally line now reads `✖ 18 problems (0 errors, 18 warnings)` (error resolved; warning count unchanged from Task 1's 18).

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/accessibility/dyslexia-provider.tsx
git commit -m "fix(web): read dyslexia preference via useSyncExternalStore

Removes the mount-effect setState that tripped react-hooks/set-state-in-effect
and caused a render-before-paint flash. Public API unchanged.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Clear the `no-unused-vars` warnings + the dead `eslint-disable`

**Files (modify):**
- `web/src/lib/fonts.ts` — remove `localFont` import (line 13)
- `web/src/app/(dashboard)/page.tsx` — remove `StatusBadge`, `AmountDisplay` imports (lines 12–13)
- `web/src/app/(dashboard)/inventory/[id]/page.tsx` — remove `isRetired` (line 344)
- `web/src/app/(dashboard)/inventory/new/page.tsx` — remove `entityLabels` (line 61)
- `web/src/app/(dashboard)/settings/page.tsx` — remove `entityLabels` (line 377), handle `values` (line 681)
- `web/src/app/(dashboard)/team/page.tsx` — remove `router` (line 175), handle `row` (line 282)
- `web/src/components/settings/role-permission-matrix.tsx` — remove `resource` (line 29)
- `web/src/components/settings/theme-customizer.tsx` — remove `resetColor` (line 110)
- `web/src/app/(platform)/verticals/page.tsx` — remove the unused `eslint-disable` directive (line 152)

**Interfaces:**
- Consumes: Task 1's `^_` override (so a dead param can be renamed `_name` instead of deleted).
- Produces: zero `@typescript-eslint/no-unused-vars` warnings and zero "unused eslint-disable directive" warnings.

**The rule for each finding:** a dead **import** or **top-level/local `const`** → delete the declaration (and, if that leaves an import specifier list empty, delete the whole import). A dead **function/callback parameter** whose position must remain (e.g. a `.map((value, index) =>` where only `index` is used) → rename it to `_name`; if it is the trailing/only param, drop it from the signature. Do NOT add `eslint-disable`.

- [ ] **Step 1: List the exact current findings**

Run (from `web/`):
```bash
npm run lint 2>&1 | grep -E "no-unused-vars|Unused eslint-disable"
```
Expected: the ~11 lines matching the files above (line numbers may have shifted by ±a few from Tasks 1–2; trust the live output over the numbers in this plan).

- [ ] **Step 2: Fix each finding**

Work top-down through the Step-1 list. For each: open the file at the reported line, apply the rule above. Two non-obvious ones:
- `settings/page.tsx:681` `'values' is defined but never used` — this is a render/callback parameter. If it is the only unused param and trails the signature, drop it; otherwise rename to `_values`.
- `team/page.tsx:282` `'row' is defined but never used` — same treatment: drop if trailing, else rename to `_row`.

For `verticals/page.tsx:152` delete only the stale `// eslint-disable-next-line …` (or inline `/* eslint-disable … */`) comment line — the code it guarded stays.

- [ ] **Step 3: Confirm all unused-vars findings are gone**

Run (from `web/`):
```bash
npm run lint 2>&1 | grep -E "no-unused-vars|Unused eslint-disable" || echo "CLEARED"
npm run lint 2>&1 | tail -1
```
Expected: first prints `CLEARED`; tally now `✖ 6 problems (0 errors, 6 warnings)` — only the 6 `react-hooks/exhaustive-deps` warnings remain for Task 4.

- [ ] **Step 4: Type-check safety net (deletions can strand a reference)**

Run (from `web/`):
```bash
npx tsc --noEmit
```
Expected: exit 0. (Catches the case where a "dead" symbol was actually referenced elsewhere; if `tsc` errors, restore that specific symbol and re-check — it was not truly unused.)

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "chore(web): remove unused imports, vars, and a dead eslint-disable

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Clear the `react-hooks/exhaustive-deps` warnings

**Files (modify):**
- `web/src/app/(dashboard)/budgets/[id]/page.tsx:79`
- `web/src/app/(dashboard)/budgets/page.tsx:45`
- `web/src/app/(dashboard)/productions/page.tsx:81`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: zero `react-hooks/exhaustive-deps` warnings → clean lint.

Each file computes `const x = <query>.data?… ?? []` and then uses `x` in one or more `useMemo` dependency arrays; the fresh `[]`/object identity each render defeats the memo. Fix: wrap the initializer in its own `useMemo` keyed on the query data. `useMemo` is already imported in all three files.

- [ ] **Step 1: Confirm the six warnings currently fire**

Run (from `web/`):
```bash
npm run lint 2>&1 | grep -c "exhaustive-deps"
```
Expected: `6`.

- [ ] **Step 2: `budgets/[id]/page.tsx` — wrap `lineItems`**

Change:
```tsx
const lineItems: BudgetLineItem[] = lineItemsQuery.data ?? [];
```
to:
```tsx
const lineItems: BudgetLineItem[] = useMemo(
  () => lineItemsQuery.data ?? [],
  [lineItemsQuery.data],
);
```

- [ ] **Step 3: `budgets/page.tsx` — wrap `productions`**

Change:
```tsx
const productions = productionsQuery.data?.productions ?? [];
```
to:
```tsx
const productions = useMemo(
  () => productionsQuery.data?.productions ?? [],
  [productionsQuery.data],
);
```

- [ ] **Step 4: `productions/page.tsx` — wrap `productions`**

Change:
```tsx
const productions = query.data?.productions ?? [];
```
to:
```tsx
const productions = useMemo(
  () => query.data?.productions ?? [],
  [query.data],
);
```

- [ ] **Step 5: Confirm exhaustive-deps is clean and lint is fully green**

Run (from `web/`):
```bash
npm run lint -- --max-warnings 0; echo "EXIT: $?"
```
Expected: no findings printed, `EXIT: 0`.

- [ ] **Step 6: Commit**

```bash
git add web/src
git commit -m "fix(web): stabilize memo deps by wrapping list initializers in useMemo

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: Add the always-run `web-lint-build` CI job

**Files:**
- Modify: `.github/workflows/ci.yml` (append a new job)

**Interfaces:**
- Consumes: Tasks 1–4 (lint must already be `--max-warnings 0` clean, build already passes).
- Produces: a required CI check `Web Lint & Build` gating `npm run lint -- --max-warnings 0` and `npm run build` on every push/PR.

- [ ] **Step 1: Confirm both gate commands pass locally**

Run (from `web/`):
```bash
npm run lint -- --max-warnings 0; echo "LINT EXIT: $?"
npm run build; echo "BUILD EXIT: $?"
```
Expected: `LINT EXIT: 0` and `BUILD EXIT: 0`.

- [ ] **Step 2: Append the job to `ci.yml`**

Add this as a new top-level job under `jobs:` in `.github/workflows/ci.yml`, after the existing `codegen-freshness` job (match the file's 2-space indentation). No `paths:` filter — it runs on every push/PR like the Go jobs and `Codegen Freshness`, so it is an unskippable required check:
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

      # --max-warnings 0: the whole point of #179 — a single new warning fails
      # the job, so web/ lint can never silently regress again (the sibling of
      # the Codegen Freshness gate for sqlc).
      - name: Lint (zero warnings allowed)
        run: npm run lint -- --max-warnings 0

      - name: Build
        run: npm run build
```

- [ ] **Step 3: Validate the workflow YAML parses**

Run (from repo root):
```bash
python3 -c "import yaml,sys; d=yaml.safe_load(open('.github/workflows/ci.yml')); assert 'web-lint-build' in d['jobs'], 'job missing'; assert d['jobs']['web-lint-build']['steps'][-1]['run']=='npm run build'; print('OK: web-lint-build present, last step is build')"
```
Expected: `OK: web-lint-build present, last step is build`.

- [ ] **Step 4: Confirm the job is NOT path-filtered or soft-failing**

Run (from repo root):
```bash
python3 -c "import yaml; j=yaml.safe_load(open('.github/workflows/ci.yml'))['jobs']['web-lint-build']; assert 'if' not in j and all('continue-on-error' not in s for s in j['steps']), 'job must not be conditional/soft'; print('OK: unconditional hard gate')"
```
Expected: `OK: unconditional hard gate`.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci(web): gate web/ lint (--max-warnings 0) and build on every PR (#179)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- §1 eslint `^_` override → Task 1 ✅
- §2 `useSyncExternalStore` error fix → Task 2 ✅
- §3 unused-vars + dead directive → Task 3 ✅
- §4 exhaustive-deps (3 named files) → Task 4 ✅
- §5 `web-lint-build` job in `ci.yml`, Node 20, `--max-warnings 0`, no path filter → Task 5 ✅
- Non-goals (no ui-e2e.yml, no Node 22, no eslint-disable, API unchanged) → encoded in Global Constraints ✅

**Placeholder scan:** none — every code step shows concrete before/after; line numbers are flagged as "trust live lint output" where drift is possible (Task 3).

**Type consistency:** `dyslexiaMode`/`toggleDyslexia`/`STORAGE_KEY`/`applyDyslexiaStyles` names match the original file (Task 2). `useMemo` wrap keeps the same variable names (`lineItems`, `productions`) so downstream `useMemo` consumers in Task 4's files are untouched.

**Ordering note:** Tasks 1→4 each reduce the lint tally toward zero; only after Task 4 does `--max-warnings 0` pass, which Task 5 depends on. Task 5 must be last.
