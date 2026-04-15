# Playwright E2E Tests

UX / end-to-end tests for the Thittam web frontend, powered by
[Playwright](https://playwright.dev/).

Tests live in `web/tests/e2e/`. The Playwright config is at
`web/playwright.config.ts`.

---

## TL;DR

```bash
# One-time (per machine)
cd web
npm install
npx playwright install --with-deps    # installs chromium/firefox/webkit + OS libs

# Run the full suite (auto-boots the Next.js dev server on :3100)
npm run test:e2e

# Interactive authoring / debugging
npm run test:e2e:ui

# Just the unauthenticated smoke tests (no backend needed)
npx playwright test --project=smoke
```

From the repo root you can also use:

```bash
make test-e2e           # runs: cd web && npm run test:e2e
make test-e2e-install   # installs deps + browsers
```

---

## Prerequisites

| Requirement           | Why                                               |
| --------------------- | ------------------------------------------------- |
| Node.js 20+           | Runs Playwright and Next.js                        |
| `web/` deps installed | `cd web && npm install`                           |
| Playwright browsers   | `npx playwright install` (chromium/firefox/webkit) |
| Backend services up   | Only for **authenticated** tests — see below      |
| Seeded DB             | Only for **authenticated** tests                  |

The `smoke` project only needs the Next.js dev server (Playwright boots it
automatically). No Postgres, no gRPC services, no seed data required.

The authenticated projects (`chromium`, `firefox`, `webkit`) need a running
backend so the login fixture can obtain a real session:

```bash
# In a separate terminal, bring up infra + services + seed the demo tenant
make dev-start
make seed
```

---

## Project layout

```
web/
├── playwright.config.ts        ← Browser projects, webServer, storageState path
├── playwright-report/          ← Generated HTML report (gitignored)
├── playwright/.auth/           ← Saved login state (gitignored)
└── tests/e2e/
    ├── auth.setup.ts           ← Logs in once, persists storageState
    ├── fixtures/auth.ts        ← Re-exports test/expect for authed specs
    ├── smoke.spec.ts           ← Unauthenticated smoke tests
    └── dashboard.spec.ts       ← Example authenticated test
```

### Playwright projects

| Project             | Auth         | Browser  | Purpose                                              |
| ------------------- | ------------ | -------- | ---------------------------------------------------- |
| `smoke`             | none         | Chromium | Fast checks that don't require a backend             |
| `setup`             | (does login) | Chromium | Runs first; writes `playwright/.auth/user.json`      |
| `chromium`          | storageState | Chromium | Full authenticated suite                             |
| `firefox`           | storageState | Firefox  | Cross-browser authenticated suite                    |
| `webkit`            | storageState | WebKit   | Cross-browser authenticated suite                    |

`chromium`/`firefox`/`webkit` declare `dependencies: ["setup"]`, so Playwright
runs `auth.setup.ts` exactly once before those projects start. Individual tests
do **not** need to log in — the storageState is reused.

---

## Running tests

### All projects

```bash
npm run test:e2e
```

### Pick a project

```bash
npx playwright test --project=smoke
npx playwright test --project=chromium
npx playwright test --project=firefox
```

### Pick a file or pattern

```bash
npx playwright test tests/e2e/smoke.spec.ts
npx playwright test -g "login page renders"
```

### Watch mode / authoring UI

```bash
npm run test:e2e:ui     # Playwright UI — time-travel debugger, test tree
```

### Headed browser (watch it happen)

```bash
npm run test:e2e:headed
```

### Open the last HTML report

```bash
npm run test:e2e:report
```

---

## Configuration

Environment variables:

| Variable                    | Default                  | Purpose                                                                         |
| --------------------------- | ------------------------ | ------------------------------------------------------------------------------- |
| `PLAYWRIGHT_PORT`           | `3100`                   | Port the web dev server listens on                                              |
| `PLAYWRIGHT_BASE_URL`       | `http://localhost:3100`  | Override baseURL (e.g. to test a preview deploy)                                |
| `PLAYWRIGHT_SKIP_WEBSERVER` | unset                    | Set to `1` when a dev server is already running — skips auto-boot               |
| `E2E_TEST_EMAIL`            | `admin@acme.test`        | Credentials used by `auth.setup.ts`                                             |
| `E2E_TEST_PASSWORD`         | `password123`            | Credentials used by `auth.setup.ts`                                             |
| `CI`                        | set by CI                | Forces `retries=2`, `workers=1`, `forbidOnly=true`, GitHub + HTML reporters     |

Example — run against a dev server you started manually:

```bash
# Terminal 1
npm run dev

# Terminal 2
PLAYWRIGHT_SKIP_WEBSERVER=1 npx playwright test --project=smoke
```

---

## Writing new tests

### Authenticated test

```ts
// tests/e2e/productions.spec.ts
import { expect, test } from "@playwright/test";

test("can navigate to productions", async ({ page }) => {
  await page.goto("/productions");
  await expect(page.getByRole("heading", { name: /productions/i })).toBeVisible();
});
```

That's it — no login code. As long as the spec is picked up by the `chromium`
(or `firefox`/`webkit`) project, Playwright loads `storageState` and the user
is already signed in.

### Unauthenticated test

Name the file to match `smoke.spec.ts` or add it to the `smoke` project's
`testMatch` in `playwright.config.ts`.

### Preferred locators (from most → least robust)

1. `page.getByRole("button", { name: /sign in/i })`
2. `page.getByLabel(/email/i)`
3. `page.getByTestId("submit-btn")` — add `data-testid` in the component
4. CSS / XPath — last resort; these break on refactors

Avoid CSS class selectors; Tailwind classes change often.

---

## Debugging a failure

When a test fails locally, Playwright auto-captures:

- **Screenshots** — `test-results/*/test-failed-*.png`
- **Video** — `test-results/*/video.webm`
- **Trace** — `test-results/*/trace.zip` (on retry)

Open the trace:

```bash
npx playwright show-trace test-results/.../trace.zip
```

The trace viewer shows the DOM snapshot at every action, network requests,
console logs, and a screencast — usually enough to diagnose without rerunning.

---

## CI

`.github/workflows/ui-e2e.yml` runs the `smoke` project on PRs that touch
`web/**`. The HTML report is uploaded as an artifact called
`playwright-report` (14-day retention).

The authenticated suite is **not** yet wired into CI — it needs a Postgres
service and the Go backend running. When added, it will live as a second job
in the same workflow with a `services: postgres: …` block (see
`migration-validate` in `ci.yml` for the pattern).

---

## Troubleshooting

| Symptom                                               | Fix                                                                                 |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `Error: browserType.launch: Executable doesn't exist` | `npx playwright install chromium` (or `--with-deps` for OS libs)                     |
| `net::ERR_CONNECTION_REFUSED` on `:3100`              | Next dev server didn't boot — check `web/` builds with `npm run dev`                |
| `setup` project fails at `waitForURL`                  | Login is failing — verify `E2E_TEST_EMAIL`/`E2E_TEST_PASSWORD` match seeded user    |
| Tests pass locally, flake in CI                       | Add an explicit `expect(...).toBeVisible()` before the action instead of `waitFor…` |
| "BEWARE: your OS is not officially supported"          | Benign on Ubuntu 24.04 — Playwright uses a fallback build                            |
