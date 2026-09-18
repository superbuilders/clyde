import { defineConfig } from "@playwright/test";

// The harness always targets a *running* instance — localhost:8788 in dev, the
// deployed HTTPS URL for a milestone gate. PLAN.md §5: a milestone is not done
// until this passes against the deployed URL, not localhost.
const baseURL = process.env.BONNIE_URL || "http://localhost:8788";

export default defineConfig({
  testDir: ".",
  // Auth is a serial story: log in once, reuse the state.
  workers: 1,
  timeout: 120_000,
  expect: { timeout: 20_000 },
  reporter: [["list"]],
  use: {
    baseURL,
    // Screenshots/traces on failure: the v1 failure mode was an empty render,
    // which is invisible in a log but obvious in a screenshot.
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: "off",
  },
  projects: [
    {
      name: "setup",
      testMatch: /auth\.setup\.ts/,
    },
    {
      name: "gate",
      testMatch: /gate\.spec\.ts/,
      dependencies: ["setup"],
      use: { storageState: "state.json" },
    },
  ],
});
