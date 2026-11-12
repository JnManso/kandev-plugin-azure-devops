import { defineConfig, devices } from "@playwright/test";

// Packaged-host contract suite. scripts/run-e2e.sh starts a disposable Kandev
// host in mock mode and sets KANDEV_PLUGIN_E2E_URL; the global setup installs
// the freshly built package into it.
export default defineConfig({
  testDir: "./ui/e2e",
  timeout: 60_000,
  expect: { timeout: 15_000 },
  workers: 1,
  globalSetup: "./ui/e2e/global-setup.mjs",
  use: {
    baseURL: process.env.KANDEV_PLUGIN_E2E_URL,
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "desktop",
      testMatch: /desktop-packaged-plugin\.spec\.ts/,
      use: { viewport: { width: 1440, height: 900 } },
    },
    {
      name: "mobile",
      testMatch: /mobile-packaged-plugin\.spec\.ts/,
      use: { ...devices["Pixel 5"] },
    },
  ],
});
