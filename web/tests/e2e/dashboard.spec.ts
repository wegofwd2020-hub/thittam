import { expect, test } from "@playwright/test";

test.describe("authenticated dashboard", () => {
  test("root redirects to an authenticated area", async ({ page }) => {
    await page.goto("/");
    await expect(page).not.toHaveURL(/\/login/);
  });
});
