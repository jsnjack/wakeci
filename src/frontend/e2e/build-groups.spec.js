import { test, expect } from "./fixtures.js";

// F2: a build started with WAKE_PARENT knows its parent, and running it again
// keeps the parent, after the parent ended too
test("a build started by another gets its parent, and run again keeps it", async ({ page, wake, backend }) => {
    const gate = await wake.gate();
    const parentJob = await wake.createJob(`tasks:\n  - name: wait\n    run: ${gate.command}\n`);
    const childJob = await wake.createJob('tasks:\n  - name: say\n    run: echo "parent=$WAKE_PARENT_ID"\n');
    const parent = await wake.runJob(parentJob);
    await wake.waitForBuild(parent, "running");
    const child = await wake.runJob(childJob, { WAKE_PARENT: String(parent) });
    await wake.waitForBuild(child);
    const log = await (await backend.api.get(`/api/build/${child}/log/0`)).text();
    expect(log).toContain(`parent=${parent}`);
    const saved = (await (await backend.api.get(`/api/build/${child}`)).json()).status_update;
    expect([saved.parent, saved.root]).toEqual([parent, parent]);

    await gate.release();
    await wake.waitForBuild(parent);
    // An ended build can't be a parent any more
    const late = await backend.api.post(`/api/job/${childJob}/run`, { form: { WAKE_PARENT: String(parent) } });
    expect(late.status()).toBe(400);
    expect(await late.text()).toContain(`build ${parent} has ended`);

    await wake.showFeed(childJob);
    const scheduled = page.waitForResponse((response) => new URL(response.url()).pathname === `/api/job/${childJob}/run`);
    await page.locator(`[data-cy-build="${child}"]`).getByTestId("run-again-button").click();
    const again = await (await scheduled).json();
    await wake.waitForBuild(again);
    const rerun = (await (await backend.api.get(`/api/build/${again}`)).json()).status_update;
    expect([rerun.parent, rerun.root]).toEqual([parent, parent]);
});
