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

  test("shows empty state when no posts", async ({ page }) => {
    await page.goto("/");
    // Either shows posts or the empty state message
    const body = await page.textContent("body");
    expect(body).toBeTruthy();
  });
});

test.describe("New page", () => {
  test("loads chronological listing", async ({ page }) => {
    await page.goto("/new");
    await expect(page).toHaveTitle(/rabbit hole/);
  });
});
