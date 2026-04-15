// Authenticated tests rely on storageState produced by auth.setup.ts
// (see playwright.config.ts project dependencies). No per-test login is
// needed — just `import { test, expect } from "@playwright/test"` in a
// chromium/firefox/webkit-project spec and the user is already signed in.
//
// Keep this module as the place to add multi-role fixtures later, e.g.:
//   export const test = base.extend<{ adminPage: Page; viewerPage: Page }>({...})
export { test, expect } from "@playwright/test";
