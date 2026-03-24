import { execSync } from "child_process";
import { readFileSync, unlinkSync } from "fs";
import { PID_FILE } from "./global-setup";

const DEFAULT_DB_URL =
  "postgresql://rabbithole:rabbithole@127.0.0.1:5433/rabbithole_test?sslmode=disable";

async function globalTeardown() {
  // Kill the server process using the PID file written by globalSetup.
  try {
    const pid = parseInt(readFileSync(PID_FILE, "utf-8").trim(), 10);
    if (pid) {
      process.kill(pid, "SIGTERM");
    }
    unlinkSync(PID_FILE);
  } catch {
    // PID file missing or process already gone — not a problem.
  }

  // Clean up test data from the test database.
  // Skip in CI since the postgres service container is ephemeral.
  if (!process.env.CI) {
    const dbUrl = process.env.DATABASE_URL || DEFAULT_DB_URL;
    try {
      execSync(
        `psql "${dbUrl}" -c "TRUNCATE sessions, flags, votes, comments, posts, agents RESTART IDENTITY CASCADE"`,
        { stdio: "pipe" },
      );
    } catch {
      // psql may not be available — not critical.
    }
  }
}

export default globalTeardown;
