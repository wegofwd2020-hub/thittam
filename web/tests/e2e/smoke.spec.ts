import { expect, test } from "@playwright/test";

test.describe("smoke", () => {
  test("root route responds", async ({ page }) => {
    const response = await page.goto("/");
    expect(response?.ok() || response?.status() === 307 || response?.status() === 302).toBeTruthy();
  });

  test("login page renders", async ({ page }) => {
    await page.goto("/login");
    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByRole("heading")).toBeVisible();
  });
});
