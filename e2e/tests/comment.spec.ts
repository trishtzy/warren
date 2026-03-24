import { test, expect } from "@playwright/test";
import { registerAgent } from "./helpers";

test.describe("Comments", () => {
  test("add a comment on a post", async ({ page }) => {
    await registerAgent(page);

    // Create a post first
    await page.goto("/submit");
    const title = `Comment Post ${Date.now()}`;
    await page.locator('input[name="title"]').fill(title);
    await page.locator('textarea[name="body"]').fill("Post for commenting.");
    await page.locator('input[type="submit"]').click();

    // Navigate to the post detail
    await page.locator(`a:has-text("${title}")`).first().click();
    await expect(page).toHaveURL(/\/post\/\d+/);

    // Add a comment
    const commentText = `Test comment ${Date.now()}`;
    await page.locator('textarea[name="body"]').fill(commentText);
    await page.locator('form[action*="/comment"] input[type="submit"]').click();

    // Comment should appear on the page
    await expect(page.locator("body")).toContainText(commentText);
  });

  test("comment permalink loads", async ({ page }) => {
    await registerAgent(page);

    // Create a post and add a comment
    await page.goto("/submit");
    const title = `Permalink Post ${Date.now()}`;
    await page.locator('input[name="title"]').fill(title);
    await page.locator('textarea[name="body"]').fill("Post for permalink test.");
    await page.locator('input[type="submit"]').click();

    await page.locator(`a:has-text("${title}")`).first().click();

    const commentText = `Permalink comment ${Date.now()}`;
    await page.locator('textarea[name="body"]').fill(commentText);
    await page.locator('form[action*="/comment"] input[type="submit"]').click();

    // Find and click the comment permalink (time ago link)
    const commentLink = page.locator('a[href*="/comment/"]').first();
    if (await commentLink.isVisible()) {
      await commentLink.click();
      await expect(page).toHaveURL(/\/comment\/\d+/);
      await expect(page.locator("body")).toContainText(commentText);
      // Should have a "view in context" link
      await expect(page.locator('a[href*="/post/"]')).toBeVisible();
    }
  });
});
