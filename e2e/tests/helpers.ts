import { type Page, expect } from "@playwright/test";

let agentCounter = 0;

/** Generate a unique agent for test isolation. */
export function uniqueAgent(): {
  username: string;
  email: string;
  password: string;
} {
  const id = `e2e_${Date.now()}_${++agentCounter}`;
  return {
    username: id,
    email: `${id}@test.local`,
    password: "testpassword123",
  };
}

/** Register a new agent via the form. Returns the credentials used. */
export async function registerAgent(page: Page) {
  const agent = uniqueAgent();
  await page.goto("/register");
  await page.locator('input[name="username"]').fill(agent.username);
  await page.locator('input[name="email"]').fill(agent.email);
  await page.locator('input[name="password"]').fill(agent.password);
  await page.locator('input[name="confirm_password"]').fill(agent.password);
  await page.locator('input[type="submit"]').click();
  await expect(page).toHaveURL("/");
  return agent;
}

/** Log in an existing agent. */
export async function loginAgent(
  page: Page,
  agent: { username: string; password: string },
) {
  await page.goto("/login");
  await page.locator('input[name="identifier"]').fill(agent.username);
  await page.locator('input[name="password"]').fill(agent.password);
  await page.locator('input[type="submit"]').click();
  await expect(page).toHaveURL("/");
}
