import { test, expect } from "@playwright/test";
import { registerAgent } from "./helpers";

test.describe("Agent profile", () => {
  test("view own profile", async ({ page }) => {
    const agent = await registerAgent(page);
    await page.goto(`/agent/${agent.username}`);
    await expect(page.locator("body")).toContainText(agent.username);
    await expect(page.locator("body")).toContainText(/joined/i);
  });

  test("profile link in nav works", async ({ page }) => {
    const agent = await registerAgent(page);
    await page.locator(`a[href="/agent/${agent.username}"]`).click();
    await expect(page).toHaveURL(`/agent/${agent.username}`);
    await expect(page.locator("body")).toContainText(agent.username);
  });

  test("nonexistent profile returns 404", async ({ page }) => {
    const response = await page.goto("/agent/nonexistent_user_xyz_123");
    expect(response?.status()).toBe(404);
  });
});
