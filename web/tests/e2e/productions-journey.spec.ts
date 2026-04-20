import { expect, test } from "@playwright/test";

test.describe("productions journey", () => {
  test("sidebar → productions list → production detail", async ({ page }) => {
    await page.goto("/");
    await expect(page).not.toHaveURL(/\/login/);

    // Sidebar label varies by vertical (Productions / Projects / Sites …);
    // match the /productions href instead of the rendered text.
    await page
      .getByRole("navigation")
      .locator('a[href="/productions"]')
      .click();

    await expect(page).toHaveURL(/\/productions\/?$/);
    await expect(
      page.getByRole("heading", { level: 1 }),
    ).toBeVisible();

    // XYZ_CBA seed — first construction project in 003_projects.sql.
    const firstProduction = page
      .getByRole("link", { name: "Oakwood Medical Plaza" })
      .first();
    await expect(firstProduction).toBeVisible();
    await firstProduction.click();

    await expect(page).toHaveURL(/\/productions\/[^/]+$/);
    await expect(
      page.getByRole("heading", { level: 1, name: "Oakwood Medical Plaza" }),
    ).toBeVisible();
  });

  test("status filter narrows the list", async ({ page }) => {
    await page.goto("/productions");

    await expect(page.getByText("Oakwood Medical Plaza")).toBeVisible();

    // Only Oakwood Medical Plaza is in "development" in the XYZ_CBA seed.
    await page.getByRole("button", { name: /^development$/i }).click();

    await expect(page.getByText("Oakwood Medical Plaza")).toBeVisible();
    await expect(page.getByText("Riverbend Logistics Hub")).toBeHidden();
  });
});
