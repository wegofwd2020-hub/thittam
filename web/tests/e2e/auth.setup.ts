import { test as setup, expect } from "@playwright/test";
import path from "node:path";

export const STORAGE_STATE = path.join(__dirname, "../../playwright/.auth/user.json");

const TEST_EMAIL = process.env.E2E_TEST_EMAIL ?? "rajesh.kumar@xyzcba.com";
const TEST_PASSWORD = process.env.E2E_TEST_PASSWORD ?? "demo1234";

setup("authenticate", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel(/email/i).fill(TEST_EMAIL);
  await page.getByLabel(/password/i).fill(TEST_PASSWORD);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 15_000 });
  await expect(page).toHaveURL(/^(?!.*\/login).*$/);
  await page.context().storageState({ path: STORAGE_STATE });
});
