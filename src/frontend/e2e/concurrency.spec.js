import { test, expect } from "./fixtures.js";

for (const concurrency of [0, 1]) {
    test(concurrency === 0 ? "should run both builds concurrently" : "should run only one build concurrently", async ({ page, wake }) => {
        const gate = await wake.gate();
        const name = await wake.createJob(`
desc: Concurrency test
tasks:
  - name: Wait for test
    run: ${gate.command}
concurrency: ${concurrency}
`);
        const first = await wake.runJob(name);
        const second = await wake.runJob(name);
        await wake.showFeed(name);
        await expect(page.locator("[data-cy-status]")).toHaveCount(2);
        await expect(page.locator(`[data-cy-build="${first}"] [data-cy-status]`)).toHaveAttribute("data-cy-status", "running");
        await expect(page.locator(`[data-cy-build="${second}"] [data-cy-status]`)).toHaveAttribute("data-cy-status", concurrency === 0 ? "running" : "pending");
        await gate.release();
        await wake.waitForBuild(first);
        await wake.waitForBuild(second);
    });
}
