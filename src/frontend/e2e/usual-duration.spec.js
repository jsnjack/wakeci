import { test, expect } from "./fixtures.js";

// L3: progress against the usual duration
test("a build far over its usual duration is marked, and flagged once it is quiet", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`params:\n  - WAIT: "no"\ntasks:\n  - name: work\n    run: if [ "$WAIT" = yes ]; then ${gate.command}; fi\n`);
    // Five finished builds give the job its usual duration
    for (let i = 0; i < 5; i++) {
        await wake.waitForBuild(await wake.runJob(name));
    }
    const id = await wake.runJob(name, { WAIT: "yes" });
    await wake.waitForBuild(id, "running");

    await page.clock.install();
    await wake.openBuild(id, "running");
    const duration = page.getByTestId("build-facts").getByTestId("duration");
    await expect(duration).toContainText(/Running for .+, usually about /);
    await expect(duration).toHaveClass(/over/);
    await expect(page.getByTestId("silent")).toHaveCount(0);
    await page.clock.fastForward("03:00");
    await expect(page.getByTestId("silent")).toHaveText(/No output for 3m/);

    await wake.showFeed(name);
    const row = page.locator(`.feed-row[data-cy-build="${id}"]`);
    await expect(row.getByTestId("duration")).toHaveClass(/over/);
    await expect(row.getByTestId("duration")).toHaveAttribute("data-tip", /^Usually about /);
    await gate.release();
});
