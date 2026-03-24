import { spawn } from "child_process";
import { writeFileSync } from "fs";
import { tmpdir } from "os";
import path from "path";

const DEFAULT_DB_URL =
  "postgresql://rabbithole:rabbithole@127.0.0.1:5433/rabbithole_test?sslmode=disable";
const PORT = "8080";
const ROOT = path.resolve(__dirname, "..");

export const PID_FILE = path.join(tmpdir(), "warren-e2e-server.pid");

async function globalSetup() {
  const dbUrl = process.env.DATABASE_URL || DEFAULT_DB_URL;

  // Start the server against the test database.
  const serverProcess = spawn("go", ["run", "./cmd/server/"], {
    cwd: ROOT,
    env: {
      ...process.env,
      DATABASE_URL: dbUrl,
      PORT,
    },
    stdio: "pipe",
  });

  // Write PID to a temp file so globalTeardown (separate process) can kill it.
  if (serverProcess.pid) {
    writeFileSync(PID_FILE, serverProcess.pid.toString(), "utf-8");
  }

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
