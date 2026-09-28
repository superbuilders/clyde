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
      testMatch: /^.*gate\.spec\.ts/,
      dependencies: ["setup"],
      use: { storageState: "state.json" },
    },
    {
      // M3 isolation. A separate project so `--project=isolation` can run the
      // milestone gate on its own without paying for the full agent round
      // trip in gate.spec.ts.
      name: "isolation",
      testMatch: /isolation\.spec\.ts/,
      dependencies: ["setup"],
      use: { storageState: "state.json" },
    },
    {
      // M4.2 share UI. The sharing project drives the HTTP API; this one
      // drives the *button*, which is the part a human actually touches and
      // the part that was missing for the whole of M4.1. It needs no AWS
      // credentials, so it can run when `sharing` cannot.
      name: "leak",
      testMatch: /leak\.spec\.ts/,
      dependencies: ["setup"],
      use: { storageState: "state.json" },
    },
    {
      name: "share-ui",
      testMatch: /share-ui\.spec\.ts/,
      dependencies: ["setup"],
      use: { storageState: "state.json" },
    },
    {
      // M4 sharing. Separate for the same reason as isolation, and because it
      // drives privileged grants over SSM: running it needs AWS credentials,
      // which the other projects do not.
      name: "sharing",
      testMatch: /sharing\.spec\.ts/,
      dependencies: ["setup"],
      use: { storageState: "state.json" },
    },
  ],
});
