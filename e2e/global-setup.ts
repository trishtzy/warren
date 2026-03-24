import { execSync, spawn, type ChildProcess } from "child_process";
import path from "path";

const TEST_DB_URL =
  "postgresql://rabbithole:rabbithole@127.0.0.1:5433/rabbithole_test?sslmode=disable";
const PORT = "8080";
const ROOT = path.resolve(__dirname, "..");

async function globalSetup() {
  // Start the server against the test database.
  const serverProcess = spawn("go", ["run", "./cmd/server/"], {
    cwd: ROOT,
    env: {
      ...process.env,
      DATABASE_URL: TEST_DB_URL,
      PORT,
    },
    stdio: "pipe",
  });

  // Store for teardown.
  (globalThis as any).__E2E_SERVER__ = serverProcess;

  // Wait for the server to be ready.
  const maxWait = 30_000;
  const start = Date.now();
  while (Date.now() - start < maxWait) {
    try {
      const res = await fetch(`http://localhost:${PORT}/`);
      if (res.ok) return;
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, 500));
  }

  throw new Error(`Server did not start within ${maxWait}ms`);
}

export default globalSetup;
