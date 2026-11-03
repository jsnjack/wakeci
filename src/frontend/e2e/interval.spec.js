import { test, expect } from "./fixtures.js";

test("should schedule a build with interval", async ({ page, wake }) => {
    const name = await wake.createJob(`
desc: Interval test
tasks:
  - name: Linux info
    run: uname -a
interval: "@every 1s"
concurrency: 1
`);
    await wake.runJob(name);
    await wake.showFeed(name);
    await expect.poll(() => page.locator('[data-cy-status="finished"]').count(), { intervals: [50, 100, 200] }).toBeGreaterThanOrEqual(2);
});
