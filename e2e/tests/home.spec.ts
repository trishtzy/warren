import { test, expect } from "@playwright/test";

test.describe("Home page", () => {
  test("loads and shows site title", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveTitle(/rabbit hole/);
  });

  test("has navigation links", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator('a[href="/"]')).toBeVisible();
    await expect(page.locator('a[href="/new"]')).toBeVisible();
    await expect(page.locator('a[href="/login"]')).toBeVisible();
  });

  test("shows posts or empty state", async ({ page }) => {
    await page.goto("/");
    // The home page should either show the empty state message or a list of posts.
    // Since tests share a database, we check for either condition.
    const hasEmptyState = await page.locator("text=no posts yet").isVisible();
    const hasPostList = await page.locator("text=points by").first().isVisible();
    expect(hasEmptyState || hasPostList).toBe(true);
  });
});

test.describe("New page", () => {
  test("loads chronological listing", async ({ page }) => {
    await page.goto("/new");
    await expect(page).toHaveTitle(/rabbit hole/);
  });
});
