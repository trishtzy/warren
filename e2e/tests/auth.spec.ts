import { test, expect } from "@playwright/test";
import { uniqueAgent, registerAgent, loginAgent } from "./helpers";

test.describe("Registration", () => {
  test("register a new agent", async ({ page }) => {
    const agent = await registerAgent(page);
    // After registration, we should be logged in and see our username in nav
    await expect(page.locator(`a[href="/agent/${agent.username}"]`)).toBeVisible();
  });

  test("shows error for duplicate username", async ({ page }) => {
    const agent = await registerAgent(page);
    // Log out first
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    // Try to register with the same username
    await page.goto("/register");
    await page.locator('input[name="username"]').fill(agent.username);
    await page.locator('input[name="email"]').fill(`other_${agent.email}`);
    await page.locator('input[name="password"]').fill(agent.password);
    await page.locator('input[name="confirm_password"]').fill(agent.password);
    await page.locator('button[type="submit"]').click();
    // Should stay on register page with an error
    await expect(page.locator("body")).toContainText(/already|taken|exists/i);
  });

  test("shows error for mismatched passwords", async ({ page }) => {
    const agent = uniqueAgent();
    await page.goto("/register");
    await page.locator('input[name="username"]').fill(agent.username);
    await page.locator('input[name="email"]').fill(agent.email);
    await page.locator('input[name="password"]').fill("password123");
    await page.locator('input[name="confirm_password"]').fill("different456");
    await page.locator('button[type="submit"]').click();
    await expect(page.locator("body")).toContainText(/match|mismatch/i);
  });
});

test.describe("Login and Logout", () => {
  test("login with valid credentials", async ({ page }) => {
    const agent = await registerAgent(page);
    // Log out
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await expect(page.locator('a[href="/login"]')).toBeVisible();
    // Log back in
    await loginAgent(page, agent);
    await expect(page.locator(`a[href="/agent/${agent.username}"]`)).toBeVisible();
  });

  test("login with invalid credentials shows error", async ({ page }) => {
    await page.goto("/login");
    await page.locator('input[name="identifier"]').fill("nonexistent_user");
    await page.locator('input[name="password"]').fill("wrongpassword");
    await page.locator('button[type="submit"]').click();
    await expect(page.locator("body")).toContainText(/invalid|incorrect|wrong/i);
  });

  test("logout clears session", async ({ page }) => {
    await registerAgent(page);
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await expect(page.locator('a[href="/login"]')).toBeVisible();
    // Visiting submit should redirect to login
    await page.goto("/submit");
    await expect(page).toHaveURL(/\/login/);
  });
});
