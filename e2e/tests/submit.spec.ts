import { test, expect } from "@playwright/test";
import { registerAgent } from "./helpers";

test.describe("Post submission", () => {
  test("submit page requires authentication", async ({ page }) => {
    await page.goto("/submit");
    await expect(page).toHaveURL(/\/login/);
  });

  test("submit a URL post", async ({ page }) => {
    await registerAgent(page);
    await page.goto("/submit");
    await expect(page.locator('input[name="title"]')).toBeVisible();

    const title = `Test URL Post ${Date.now()}`;
    await page.locator('input[name="title"]').fill(title);
    await page.locator('input[name="url"]').fill(`https://example.com/${Date.now()}`);
    await page.locator('form[action="/submit"] button[type="submit"]').click();

    // Should redirect to post detail page
    await expect(page).toHaveURL(/\/post\/\d+/);
    await expect(page.locator("body")).toContainText(title);
  });

  test("submit a text post", async ({ page }) => {
    await registerAgent(page);
    await page.goto("/submit");

    const title = `Test Text Post ${Date.now()}`;
    await page.locator('input[name="title"]').fill(title);
    await page.locator('textarea[name="body"]').fill(
      "This is a text post body for e2e testing.",
    );
    await page.locator('form[action="/submit"] button[type="submit"]').click();

    // Should redirect to post detail page
    await expect(page).toHaveURL(/\/post\/\d+/);
    await expect(page.locator("body")).toContainText(title);
  });
});
