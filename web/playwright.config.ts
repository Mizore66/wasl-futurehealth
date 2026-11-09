import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.PORT ?? 3100);
const external = process.env.PLAYWRIGHT_BASE_URL;

const chrome = { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } };

export default defineConfig({
  testDir: "e2e",
  // Screens go to test-results/screens; Playwright cleans only outputDir, so keep them apart.
  outputDir: "test-results/artifacts",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  // More workers than this starve next start on a laptop and clicks land before hydration.
  workers: 3,
  reporter: process.env.CI ? [["github"], ["list"], ["html", { open: "never" }]] : [["list"]],
  use: {
    baseURL: external ?? `http://localhost:${port}`,
    viewport: { width: 1440, height: 900 },
    trace: "retain-on-failure",
    // Sandboxes that preinstall a different Chromium build point at it here instead of running `playwright install`.
    launchOptions: process.env.PW_CHROMIUM_PATH ? { executablePath: process.env.PW_CHROMIUM_PATH } : {},
  },
  projects: [
    { name: "chromium", testIgnore: ["site.spec.ts", "patients-perf.spec.ts"], use: chrome },
    // The landing's WebGL stage draws on its own clock, so parallel pages starve each other and time out.
    { name: "landing", testMatch: "site.spec.ts", workers: 1, dependencies: ["chromium"], use: chrome },
    // Long-task timings at 4x CPU throttle depend on what else is running, so this spec runs alone, after the rest.
    { name: "perf", testMatch: "patients-perf.spec.ts", workers: 1, dependencies: ["chromium", "landing"], use: chrome },
  ],
  webServer: external
    ? undefined
    : { command: `pnpm exec next start -p ${port}`, url: `http://localhost:${port}/app`, reuseExistingServer: false, timeout: 120_000 },
});
