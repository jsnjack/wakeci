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

    // From the build page, through the run form
    await wake.openBuild(child);
    await page.getByTestId("run-again-button").click();
    const scheduled = page.waitForResponse((response) => new URL(response.url()).pathname === `/api/job/${childJob}/run`);
    await page.getByTestId("start-job-confirm").click();
    const again = await (await scheduled).json();
    await wake.waitForBuild(again);
    const rerun = (await (await backend.api.get(`/api/build/${again}`)).json()).status_update;
    expect([rerun.parent, rerun.root]).toEqual([parent, parent]);
});

test("the build page shows the group: the tree on the root, the strip on a member", async ({ page, wake }) => {
    const rootGate = await wake.gate();
    const childGate = await wake.gate();
    const rootJob = await wake.createJob(`tasks:\n  - name: wait\n    run: ${rootGate.command}\n`);
    const waitJob = await wake.createJob(`tasks:\n  - name: wait\n    run: ${childGate.command}\n`);
    const quickJob = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    const root = await wake.runJob(rootJob);
    await wake.waitForBuild(root, "running");
    const first = await wake.runJob(waitJob, { WAKE_PARENT: String(root) });
    await wake.waitForBuild(first, "running");
    // Queued: the gated builds take both slots
    const second = await wake.runJob(quickJob, { WAKE_PARENT: String(root) });
    const grandchild = await wake.runJob(quickJob, { WAKE_PARENT: String(first) });

    // The root lists them as a tree, and a new one as soon as it starts
    await wake.livePage(`/build/${root}`);
    const group = page.getByTestId("build-group");
    await expect(group.getByRole("heading")).toHaveText("Started 3 builds");
    await expect(group.getByTestId("group-build")).toHaveText([
        new RegExp(`#${first}${waitJob}`),
        new RegExp(`#${grandchild}${quickJob}`),
        new RegExp(`#${second}${quickJob}`),
    ]);
    const third = await wake.runJob(quickJob, { WAKE_PARENT: String(root) });
    await expect(group.getByRole("heading")).toHaveText("Started 4 builds");
    await expect(group.getByTestId("group-build").last()).toContainText(`#${third}`);

    // A member says what it is part of; [ and ] move between the builds its
    // parent started
    await childGate.release();
    await rootGate.release();
    for (const id of [root, first, grandchild, third]) {
        await wake.waitForBuild(id);
    }
    await wake.openBuild(second);
    const strip = page.getByTestId("build-group");
    await expect(strip.getByTestId("group-root")).toHaveText(`#${root} ${rootJob}`);
    await expect(strip.getByTestId("group-build")).toHaveCount(5);
    await expect(strip.locator('[aria-current="page"]')).toHaveAttribute("aria-label", new RegExp(`^#${second} `));
    await page.keyboard.press("[");
    await expect(page).toHaveURL(`/build/${first}`);
    await expect(page.getByTestId("page-title")).toContainText(`#${first}`);
    await page.keyboard.press("]");
    await expect(page).toHaveURL(`/build/${second}`);
    await strip.getByTestId("group-root").click();
    await expect(page).toHaveURL(`/build/${root}`);
    await expect(page.getByTestId("build-group").getByRole("heading")).toHaveText("Started 4 builds");
});

test("the feed shows a group under its root, live, and aborts it all", async ({ page, wake }) => {
    const rootGate = await wake.gate();
    const lastGate = await wake.gate();
    const rootJob = await wake.createJob(`tasks:\n  - name: wait\n    run: ${rootGate.command}\n`);
    const passJob = await wake.createJob("tasks:\n  - name: pass\n    run: echo ok\n");
    const failJob = await wake.createJob("tasks:\n  - name: fail\n    run: exit 1\n");
    const waitJob = await wake.createJob(`tasks:\n  - name: wait\n    run: ${lastGate.command}\n`);
    const root = await wake.runJob(rootJob);
    await wake.waitForBuild(root, "running");
    const passed = await wake.runJob(passJob, { WAKE_PARENT: String(root) });
    await wake.waitForBuild(passed);

    // A search for the root's job shows the group
    await wake.showFeed(rootJob);
    const row = page.locator(`.feed-row[data-cy-build="${root}"]`);
    const lines = page.locator(`[data-cy-group="${root}"]`).getByTestId("group-line");
    await expect(row.locator("[data-cy-status]")).toHaveAttribute("data-cy-status", "running");
    await expect(lines).toHaveCount(1);
    await expect(lines.first()).toContainText(`#${passed}`);

    // New builds join it live; a failure shows on the root's row at once
    const failed = await wake.runJob(failJob, { WAKE_PARENT: String(root) });
    await wake.waitForBuild(failed, "failed");
    await expect(lines).toHaveCount(2);
    await expect(row.getByTestId("group-failed")).toHaveText("1 failed");

    // Run again shows the latest attempt only
    const again = await wake.runJob(failJob, { WAKE_AGAIN: String(failed) });
    await wake.waitForBuild(again, "failed");
    await expect(lines).toHaveCount(2);
    await expect(page.locator(`[data-cy-group="${root}"] [data-cy-build="${again}"]`)).toHaveAttribute("data-tip", "2 attempts");

    const last = await wake.runJob(waitJob, { WAKE_PARENT: String(root) });
    await wake.waitForBuild(last, "running");
    await row.getByTestId("abort-build-button").click();
    await wake.waitForBuild(root, "aborted");
    await wake.waitForBuild(last, "aborted");
    // Ended: the root shows the worst of all
    await expect(row.locator("[data-cy-status]")).toHaveAttribute("data-cy-status", "failed");
    await expect(row.getByTestId("group-failed")).toHaveCount(0);
    await rootGate.release();
    await lastGate.release();
});
