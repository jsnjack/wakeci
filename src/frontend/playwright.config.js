import { availableParallelism } from "node:os";
import { defineConfig } from "@playwright/test";

export default defineConfig({
    testDir: "./e2e",
    // The screenshots are compared only by `make screens`, which takes the
    // baselines from a build of the base commit first
    testIgnore: process.env.E2E_SCREENS ? [] : ["**/screens.spec.js"],
    fullyParallel: true,
    workers: Math.min(10, Math.max(1, Math.floor(availableParallelism() / 2))),
    forbidOnly: !!process.env.CI,
    retries: 0,
    timeout: 30000,
    // Baselines stay out of git, in screens/ (`make screens`)
    expect: { timeout: 10000, toHaveScreenshot: { pathTemplate: "screens/{arg}{ext}" } },
    reporter: [["list"], ["html", { open: "never" }]],
    use: {
        browserName: "chromium",
        channel: process.env.E2E_BROWSER_CHANNEL || "chrome",
        viewport: { width: 1280, height: 720 },
        testIdAttribute: "data-cy",
        trace: "retain-on-failure",
        screenshot: "only-on-failure",
    },
});
