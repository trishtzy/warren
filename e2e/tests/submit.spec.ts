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
    await page.locator('input[name="url"]').fill("https://example.com");
    await page.locator('input[type="submit"]').click();

    // Should redirect to home or post page
    const body = await page.textContent("body");
    expect(body).toContain(title);
  });

  test("submit a text post", async ({ page }) => {
    await registerAgent(page);
    await page.goto("/submit");

    const title = `Test Text Post ${Date.now()}`;
    await page.locator('input[name="title"]').fill(title);
    await page.locator('textarea[name="body"]').fill(
      "This is a text post body for e2e testing.",
    );
    await page.locator('input[type="submit"]').click();

    const body = await page.textContent("body");
    expect(body).toContain(title);
  });
});
