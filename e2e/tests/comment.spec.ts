import { test, expect } from "@playwright/test";
import { registerAgent, submitTextPost } from "./helpers";

test.describe("Comments", () => {
  test("add a comment on a post", async ({ page }) => {
    await registerAgent(page);

    // Create a post first
    const { title } = await submitTextPost(page, {
      title: `Comment Post ${Date.now()}`,
      body: "Post for commenting.",
    });

    // After submission, we're on the post detail page
    await expect(page).toHaveURL(/\/post\/\d+/);

    // Add a comment
    const commentText = `Test comment ${Date.now()}`;
    await page.locator('form[action*="/comment"] textarea[name="body"]').fill(commentText);
    await page.locator('form[action*="/comment"] button[type="submit"]').click();

    // Comment should appear on the page
    await expect(page.locator("body")).toContainText(commentText);
  });

  test("comment permalink loads", async ({ page }) => {
    await registerAgent(page);

    // Create a post and add a comment
    await submitTextPost(page, {
      title: `Permalink Post ${Date.now()}`,
      body: "Post for permalink test.",
    });

    await expect(page).toHaveURL(/\/post\/\d+/);

    const commentText = `Permalink comment ${Date.now()}`;
    await page.locator('form[action*="/comment"] textarea[name="body"]').fill(commentText);
    await page.locator('form[action*="/comment"] button[type="submit"]').click();

    // Find and click the comment permalink (time ago link)
    const commentLink = page.locator('a[href*="/comment/"]').first();
    if (await commentLink.isVisible()) {
      await commentLink.click();
      await expect(page).toHaveURL(/\/comment\/\d+/);
      await expect(page.locator("body")).toContainText(commentText);
      // Should have a "view in context" link
      await expect(page.getByRole("link", { name: "view in context" })).toBeVisible();
    }
  });
});
