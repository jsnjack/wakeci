import { test, expect } from "./fixtures.js";

// cancel.go: a newer build cancels the older one with its cancel_group key,
// and with the same concurrency_key it waits until the older one's finally
// task has cleaned up
test("a newer build cancels the older one and waits for its cleanup", async ({ wake, backend }) => {
    const work = await wake.gate();
    const cleanup = await wake.gate();
    const job = await wake.createJob(
        'params:\n  - NAME: ""\n' +
            "cancel_group: pr/$NAME\nconcurrency_key: pr/$NAME\n" +
            `tasks:\n  - name: work\n    run: ${work.command}\n` +
            `finally:\n  - name: clean up\n    run: ${cleanup.command}\n`,
    );
    const older = await wake.runJob(job, { NAME: "pr1" });
    await wake.waitForBuild(older, "running");
    const other = await wake.runJob(job, { NAME: "pr2" });
    await wake.waitForBuild(other, "running");

    const newer = await wake.runJob(job, { NAME: "pr1" });
    await wake.waitForBuild(older, "aborted");
    const log = await (await backend.api.get(`/api/build/${older}/log/0`)).text();
    expect(log).toContain(`Aborted as superseded by #${newer} (pr/pr1).`);
    // The older build's finally task still runs, and holds the key
    const status = async (id) => (await (await backend.api.get(`/api/build/${id}`)).json()).status_update.status;
    await new Promise((resolve) => setTimeout(resolve, 500));
    expect(await status(newer)).toBe("pending");
    expect(await status(other)).toBe("running");

    await cleanup.release();
    await wake.waitForBuild(newer, "running");
    await work.release();
    await wake.waitForBuild(newer);
    await wake.waitForBuild(other);
});
