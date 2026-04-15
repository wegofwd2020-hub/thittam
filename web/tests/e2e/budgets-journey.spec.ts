import { expect, test } from "@playwright/test";

test.describe("budgets journey", () => {
  test("sidebar → budgets list → budget detail", async ({ page }) => {
    await page.goto("/");
    await expect(page).not.toHaveURL(/\/login/);

    await page
      .getByRole("navigation")
      .getByRole("link", { name: /^budgets$/i })
      .click();

    await expect(page).toHaveURL(/\/budgets\/?$/);
    await expect(page.getByRole("heading", { level: 1, name: /budgets/i })).toBeVisible();

    const firstBudget = page.getByRole("link", { name: "The Last Horizon V1" }).first();
    await expect(firstBudget).toBeVisible();
    await firstBudget.click();

    await expect(page).toHaveURL(/\/budgets\/[^/]+$/);
  });

  test("status filter narrows the list", async ({ page }) => {
    await page.goto("/budgets");

    await expect(page.getByText("The Last Horizon V1")).toBeVisible();
    await expect(page.getByText("Midnight Express V1")).toBeVisible();

    await page.getByRole("button", { name: /^draft$/i }).click();

    await expect(page.getByText("The Last Horizon V2")).toBeVisible();
    await expect(page.getByText("Midnight Express V1")).toBeHidden();
  });
});
