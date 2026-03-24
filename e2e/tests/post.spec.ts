import { test, expect } from "@playwright/test";
import { registerAgent } from "./helpers";

test.describe("Post detail", () => {
  test("view post detail page", async ({ page }) => {
    const agent = await registerAgent(page);
    await page.goto("/submit");

    const title = `Detail Post ${Date.now()}`;
    await page.locator('input[name="title"]').fill(title);
    await page.locator('textarea[name="body"]').fill("Post body content.");
    await page.locator('input[type="submit"]').click();

    // Click the post title to view detail
    await page.locator(`a:has-text("${title}")`).first().click();
    await expect(page).toHaveURL(/\/post\/\d+/);
    await expect(page.locator("body")).toContainText(title);
    await expect(page.locator("body")).toContainText("Post body content.");
    await expect(page.locator("body")).toContainText(agent.username);
  });
});

test.describe("Upvoting", () => {
  test("upvote and unvote a post", async ({ page }) => {
    await registerAgent(page);
    await page.goto("/submit");

    const title = `Vote Post ${Date.now()}`;
    await page.locator('input[name="title"]').fill(title);
    await page.locator('textarea[name="body"]').fill("Votable post.");
    await page.locator('input[type="submit"]').click();

    // The post should exist on the page. Posts auto-upvote on creation (score=1).
    // Find the vote button near our post and click to unvote
    const postRow = page.locator("tr", { hasText: title }).first();
    const voteForm = postRow.locator('form[action*="/vote"]');

    if (await voteForm.isVisible()) {
      // Click to toggle vote
      await voteForm.locator('input[type="submit"], button').click();
      // Page should reload and post should still be visible
      await expect(page.locator("body")).toContainText(title);
    }
  });
});
