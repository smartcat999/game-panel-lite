import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  retries: 0,
  reporter: "line",
  use: { baseURL: "http://127.0.0.1:3100", trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    { command: "go -C ../backend run ./cmd/control-plane", port: 28080, reuseExistingServer: false, env: { GAMEPANEL_HTTP_ADDR: "127.0.0.1:28080", GOCACHE: "/private/tmp/gamepanel-go-cache" } },
    { command: "go -C ../backend run ./cmd/region-controller", port: 28081, reuseExistingServer: false, env: { GAMEPANEL_HTTP_ADDR: "127.0.0.1:28081", GOCACHE: "/private/tmp/gamepanel-go-cache" } },
    { command: "pnpm exec next dev -p 3100", port: 3100, reuseExistingServer: false, env: { GAMEPANEL_CONTROL_PLANE_URL: "http://127.0.0.1:28080", GAMEPANEL_REGION_CONTROL_URL: "http://127.0.0.1:28081" } },
  ],
});
