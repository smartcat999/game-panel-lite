import { defineConfig, devices } from "@playwright/test";

// Dedicated #32 P2 browser validation port; override remains available to CI.
const port = Number(process.env.PLAYWRIGHT_PORT ?? 31833);

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  retries: 0,
  reporter: "line",
  use: { baseURL: `http://127.0.0.1:${port}`, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: { command: `pnpm exec next ${process.env.PLAYWRIGHT_PRODUCTION === "1" ? "start" : "dev"} -p ${port}`, port, reuseExistingServer: false },
});
