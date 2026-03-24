import { test, expect, type Page } from "@playwright/test";
import { registerAgent, loginAgent, submitTextPost } from "./helpers";
import { execSync } from "child_process";

const DEFAULT_DB_URL =
  "postgresql://rabbithole:rabbithole@127.0.0.1:5433/rabbithole_test?sslmode=disable";

/** Run a SQL statement against the test database. */
function sql(query: string): string {
  const dbUrl = process.env.DATABASE_URL || DEFAULT_DB_URL;
  return execSync(`psql "${dbUrl}" -t -A -c "${query}"`, {
    encoding: "utf-8",
  }).trim();
}

/** Promote an agent to admin by username. */
function promoteToAdmin(username: string) {
  sql(`UPDATE agents SET is_admin = TRUE WHERE username = '${username}'`);
}

/** Get the CSRF token from a page's hidden input. */
async function getCSRFToken(page: Page): Promise<string> {
  const token = await page
    .locator('input[name="csrf_token"]')
    .first()
    .getAttribute("value");
  return token ?? "";
}

/**
 * Extract the post ID from a /post/:id URL.
 */
function postIDFromURL(url: string): string {
  const match = url.match(/\/post\/(\d+)/);
  return match ? match[1] : "";
}

test.describe("Flagging", () => {
  test("authenticated user can flag a post", async ({ page }) => {
    const agent = await registerAgent(page);
    const { title } = await submitTextPost(page, {
      title: `Flag Post ${Date.now()}`,
      body: "Post to be flagged.",
    });

    // We are on /post/:id after submission.
    const postID = postIDFromURL(page.url());
    expect(postID).toBeTruthy();

    // Flag the post via direct POST request (flag button may not be in UI yet).
    const csrfToken = await getCSRFToken(page);
    const response = await page.request.post(`/post/${postID}/flag`, {
      form: { csrf_token: csrfToken },
    });

    // Should redirect back to the post page (303).
    expect(response.status()).toBe(200); // after redirect
    expect(response.url()).toContain(`/post/${postID}`);

    // Verify the flag was recorded: promote agent to admin and check moderation page.
    promoteToAdmin(agent.username);

    // Re-login to pick up admin status.
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, agent);

    await page.goto("/admin/moderation");
    await expect(page.locator("body")).toContainText(title);
  });

  test("authenticated user can flag a comment", async ({ page }) => {
    const agent = await registerAgent(page);
    await submitTextPost(page, {
      title: `Comment Flag Post ${Date.now()}`,
      body: "Post with a comment to flag.",
    });

    const postID = postIDFromURL(page.url());

    // Add a comment.
    const commentText = `Flaggable comment ${Date.now()}`;
    await page
      .locator('form[action*="/comment"] textarea[name="body"]')
      .fill(commentText);
    await page
      .locator('form[action*="/comment"] button[type="submit"]')
      .click();
    await expect(page.locator("body")).toContainText(commentText);

    // Get the comment ID from the comment permalink.
    const commentLink = page.locator('a[href*="/comment/"]').first();
    const href = await commentLink.getAttribute("href");
    const commentID = href?.match(/\/comment\/(\d+)/)?.[1];
    expect(commentID).toBeTruthy();

    // Flag the comment via direct POST request.
    const csrfToken = await getCSRFToken(page);
    const response = await page.request.post(`/comment/${commentID}/flag`, {
      form: { csrf_token: csrfToken, post_id: postID },
    });
    expect(response.status()).toBe(200); // after redirect

    // Verify via admin moderation page.
    promoteToAdmin(agent.username);
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, agent);

    await page.goto("/admin/moderation");
    await expect(page.locator("body")).toContainText(commentText);
  });

  test("unauthenticated user cannot flag a post", async ({ page }) => {
    // First create a post as an authenticated user.
    const agent = await registerAgent(page);
    await submitTextPost(page, {
      title: `Unauth Flag Post ${Date.now()}`,
      body: "Should not be flaggable without auth.",
    });
    const postID = postIDFromURL(page.url());

    // Log out.
    await page.locator('form[action="/logout"] button[type="submit"]').click();

    // Try to flag the post — should redirect to login.
    const response = await page.request.post(`/post/${postID}/flag`, {
      form: {},
      maxRedirects: 0,
    });
    expect(response.status()).toBe(303);
    expect(response.headers()["location"]).toContain("/login");
  });
});

test.describe("Admin moderation page access", () => {
  test("admin can access /admin/moderation", async ({ page }) => {
    const agent = await registerAgent(page);
    promoteToAdmin(agent.username);

    // Re-login to pick up admin status.
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, agent);

    await page.goto("/admin/moderation");
    await expect(page.locator("body")).toContainText("Moderation Dashboard");
  });

  test("non-admin cannot access /admin/moderation", async ({ page }) => {
    await registerAgent(page);

    const response = await page.goto("/admin/moderation");
    expect(response?.status()).toBe(403);
  });

  test("unauthenticated user is redirected from /admin/moderation", async ({
    page,
  }) => {
    await page.goto("/admin/moderation");
    await expect(page).toHaveURL(/\/login/);
  });
});

test.describe("Admin hide/unhide post", () => {
  test("admin can hide a post and it disappears from front page", async ({
    page,
  }) => {
    // Create a post as a regular user.
    const user = await registerAgent(page);
    const { title } = await submitTextPost(page, {
      title: `Hideable Post ${Date.now()}`,
      body: "This post will be hidden.",
    });
    const postID = postIDFromURL(page.url());

    // Flag the post so it shows up on the moderation page.
    const csrfToken = await getCSRFToken(page);
    await page.request.post(`/post/${postID}/flag`, {
      form: { csrf_token: csrfToken },
    });

    // Promote user to admin and re-login.
    promoteToAdmin(user.username);
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, user);

    // Go to moderation page and hide the post.
    await page.goto("/admin/moderation");
    await expect(page.locator("body")).toContainText(title);

    // Find the hide button for this post and click it.
    const postRow = page.locator("tr", { hasText: title });
    await postRow
      .locator('form[action="/admin/moderation/hide-post"] button')
      .click();

    // Should redirect back to moderation page with success message.
    await expect(page.locator("body")).toContainText(`Post ${postID} hidden`);

    // The post should now show "hidden" status on moderation page.
    const updatedRow = page.locator("tr", { hasText: title });
    await expect(updatedRow).toContainText("hidden");

    // The post should no longer appear on the front page.
    await page.goto("/");
    const frontPageContent = await page.locator("body").textContent();
    expect(frontPageContent).not.toContain(title);

    // The post detail page should return 404 (hidden = not found).
    const detailResponse = await page.goto(`/post/${postID}`);
    expect(detailResponse?.status()).toBe(404);
  });

  test("admin can unhide a post and it reappears", async ({ page }) => {
    // Create and flag a post.
    const user = await registerAgent(page);
    const { title } = await submitTextPost(page, {
      title: `Unhideable Post ${Date.now()}`,
      body: "This post will be hidden then unhidden.",
    });
    const postID = postIDFromURL(page.url());

    const csrfToken = await getCSRFToken(page);
    await page.request.post(`/post/${postID}/flag`, {
      form: { csrf_token: csrfToken },
    });

    // Promote to admin and re-login.
    promoteToAdmin(user.username);
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, user);

    // Hide the post via moderation page.
    await page.goto("/admin/moderation");
    const postRow = page.locator("tr", { hasText: title });
    await postRow
      .locator('form[action="/admin/moderation/hide-post"] button')
      .click();
    await expect(page.locator("body")).toContainText(`Post ${postID} hidden`);

    // Now unhide the post.
    const hiddenRow = page.locator("tr", { hasText: title });
    await hiddenRow
      .locator('form[action="/admin/moderation/unhide-post"] button')
      .click();
    await expect(page.locator("body")).toContainText(
      `Post ${postID} unhidden`,
    );

    // The post should reappear on the front page.
    await page.goto("/");
    await expect(page.locator("body")).toContainText(title);

    // The post detail page should be accessible again.
    const detailResponse = await page.goto(`/post/${postID}`);
    expect(detailResponse?.status()).toBe(200);
  });
});

test.describe("Admin ban/unban agent", () => {
  test("admin can ban an agent and the agent cannot log in", async ({
    page,
    context,
  }) => {
    // Register the agent who will be banned.
    const victim = await registerAgent(page);
    const { title } = await submitTextPost(page, {
      title: `Ban Victim Post ${Date.now()}`,
      body: "Post by agent who will be banned.",
    });
    const postID = postIDFromURL(page.url());

    // Flag the post so it appears on moderation page.
    const csrfToken = await getCSRFToken(page);
    await page.request.post(`/post/${postID}/flag`, {
      form: { csrf_token: csrfToken },
    });

    // Log out victim.
    await page.locator('form[action="/logout"] button[type="submit"]').click();

    // Register an admin user.
    const admin = await registerAgent(page);
    promoteToAdmin(admin.username);
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, admin);

    // Go to moderation page and ban the agent.
    await page.goto("/admin/moderation");
    await expect(page.locator("body")).toContainText(title);

    // Find the "ban user" button in the row with the victim's post.
    const postRow = page.locator("tr", { hasText: title });
    // Accept the confirmation dialog.
    page.on("dialog", (dialog) => dialog.accept());
    await postRow
      .locator('form[action="/admin/moderation/ban-agent"] button')
      .click();

    // Should show success message.
    await expect(page.locator("body")).toContainText(/banned/i);

    // Log out the admin.
    await page.locator('form[action="/logout"] button[type="submit"]').click();

    // Try to log in as the banned agent — should fail.
    await page.goto("/login");
    await page.locator('input[name="identifier"]').fill(victim.username);
    await page.locator('input[name="password"]').fill(victim.password);
    await page.locator('button[type="submit"]').click();

    // Should stay on login page with an error.
    await expect(page.locator("body")).toContainText(
      /invalid|incorrect|wrong/i,
    );
  });

  test("admin can unban an agent and the agent can log in again", async ({
    page,
  }) => {
    // Register the agent who will be banned then unbanned.
    const victim = await registerAgent(page);
    const { title } = await submitTextPost(page, {
      title: `Unban Victim Post ${Date.now()}`,
      body: "Post by agent who will be banned then unbanned.",
    });
    const postID = postIDFromURL(page.url());

    // Flag the post.
    const csrfToken = await getCSRFToken(page);
    await page.request.post(`/post/${postID}/flag`, {
      form: { csrf_token: csrfToken },
    });

    // Log out victim.
    await page.locator('form[action="/logout"] button[type="submit"]').click();

    // Register admin and ban the victim.
    const admin = await registerAgent(page);
    promoteToAdmin(admin.username);
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, admin);

    await page.goto("/admin/moderation");
    const postRow = page.locator("tr", { hasText: title });
    page.on("dialog", (dialog) => dialog.accept());
    await postRow
      .locator('form[action="/admin/moderation/ban-agent"] button')
      .click();
    await expect(page.locator("body")).toContainText(/banned/i);

    // Now unban the victim via direct SQL (since the UI shows "ban" not "unban"
    // on the moderation page for agents -- unban is done by agent_id).
    // Get the victim's agent_id.
    const victimID = sql(
      `SELECT id FROM agents WHERE username = '${victim.username}'`,
    );

    // Use the admin session to unban via POST.
    const modCSRF = await getCSRFToken(page);
    await page.request.post("/admin/moderation/unban-agent", {
      form: { csrf_token: modCSRF, agent_id: victimID },
    });

    // Log out admin.
    await page.locator('form[action="/logout"] button[type="submit"]').click();

    // The victim should be able to log in again.
    await loginAgent(page, victim);
    await expect(
      page.locator(`a[href="/agent/${victim.username}"]`),
    ).toBeVisible();
  });
});

test.describe("Moderation log", () => {
  test("admin actions are recorded in the moderation log", async ({
    page,
  }) => {
    // Create a post, flag it, then hide it as admin.
    const user = await registerAgent(page);
    const { title } = await submitTextPost(page, {
      title: `Mod Log Post ${Date.now()}`,
      body: "Post to verify moderation log.",
    });
    const postID = postIDFromURL(page.url());

    const csrfToken = await getCSRFToken(page);
    await page.request.post(`/post/${postID}/flag`, {
      form: { csrf_token: csrfToken },
    });

    promoteToAdmin(user.username);
    await page.locator('form[action="/logout"] button[type="submit"]').click();
    await loginAgent(page, user);

    // Hide the post.
    await page.goto("/admin/moderation");
    const postRow = page.locator("tr", { hasText: title });
    await postRow
      .locator('form[action="/admin/moderation/hide-post"] button')
      .click();

    // The moderation log section should contain the hide action.
    await expect(page.locator("body")).toContainText("Moderation Log");
    await expect(page.locator("body")).toContainText("hide_post");
  });
});
