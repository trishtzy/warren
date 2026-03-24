import { execSync, type ChildProcess } from "child_process";

const TEST_DB_URL =
  "postgresql://rabbithole:rabbithole@127.0.0.1:5433/rabbithole_test?sslmode=disable";

async function globalTeardown() {
  // Kill the server process.
  const serverProcess = (globalThis as any).__E2E_SERVER__ as
    | ChildProcess
    | undefined;
  if (serverProcess?.pid) {
    serverProcess.kill("SIGTERM");
  }

  // Clean up test data from the test database.
  execSync(
    `psql "${TEST_DB_URL}" -c "TRUNCATE sessions, flags, votes, comments, posts, agents RESTART IDENTITY CASCADE"`,
    { stdio: "pipe" },
  );
}

export default globalTeardown;
