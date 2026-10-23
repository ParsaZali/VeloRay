import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "tests",
  testMatch: ["panel-refinements.spec.ts", "commerce-workflow.spec.ts"],
  workers: 1,
  retries: 0,
  use: {
    baseURL: "http://127.0.0.1:8861",
    headless: true,
    viewport: { width: 1440, height: 1000 },
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "npm run dev -- --host 127.0.0.1 --port 8861",
    url: "http://127.0.0.1:8861",
    reuseExistingServer: false,
  },
  reporter: [["list"]],
});
