import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.PORT ?? 3100);
const external = process.env.PLAYWRIGHT_BASE_URL;

const chrome = { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } };

export default defineConfig({
  testDir: "e2e",
  // Screens go to test-results/screens; Playwright cleans only outputDir, so keep them apart.
  outputDir: "test-results/artifacts",
  fullyParallel: true,
  // More workers than this starve next start on a laptop and clicks land before hydration.
  workers: 3,
  reporter: [["list"]],
  use: {
    baseURL: external ?? `http://localhost:${port}`,
    viewport: { width: 1440, height: 900 },
    trace: "retain-on-failure",
  },
  projects: [
    { name: "chromium", testIgnore: "site.spec.ts", use: chrome },
    // The landing's WebGL stage draws on its own clock, so parallel pages starve each other and time out.
    { name: "landing", testMatch: "site.spec.ts", workers: 1, dependencies: ["chromium"], use: chrome },
  ],
  webServer: external
    ? undefined
    : { command: `pnpm exec next start -p ${port}`, url: `http://localhost:${port}/app`, reuseExistingServer: false, timeout: 120_000 },
});
