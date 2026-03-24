import { test, expect } from "@playwright/test";
import { registerAgent, submitTextPost } from "./helpers";

test.describe("Post detail", () => {
  test("view post detail page", async ({ page }) => {
    const agent = await registerAgent(page);
    const { title, body } = await submitTextPost(page, {
      title: `Detail Post ${Date.now()}`,
      body: "Post body content.",
    });

    // After submission we're redirected to post detail page
    await expect(page).toHaveURL(/\/post\/\d+/);
    await expect(page.locator("body")).toContainText(title);
    await expect(page.locator("body")).toContainText("Post body content.");
    await expect(page.locator("body")).toContainText(agent.username);
  });
});

test.describe("Upvoting", () => {
  test("upvote and unvote a post", async ({ page }) => {
    await registerAgent(page);
    const { title } = await submitTextPost(page, {
      title: `Vote Post ${Date.now()}`,
      body: "Votable post.",
    });

    // After submission, we're on the post detail page.
    // The post auto-upvotes on creation (score=1).
    // Find the vote button and click to unvote (toggle).
    const voteForm = page.locator('form[action*="/vote"]');
    await expect(voteForm).toBeVisible();
    await voteForm.locator("button[type='submit']").click();
    // Page should reload and post should still be visible
    await expect(page.locator("body")).toContainText(title);
  });
});
