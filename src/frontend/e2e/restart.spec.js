import { access, readdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { test, expect } from "./fixtures.js";

// These tests stop wakeci for real and start it again on the same port and
// directories (the restartable fixture), so the database carries over

// waitFor returns a shell command that waits until file exists
function waitFor(file) {
    return `while [ ! -f '${file}' ]; do sleep 0.05; done`;
}

// taskLogs returns the task logs of a build, as wakeci wrote them
async function taskLogs(root, id) {
    const dir = join(root, "work", "wakespace", String(id));
    const names = (await readdir(dir)).filter((name) => name.startsWith("task_"));
    return (await Promise.all(names.map((name) => readFile(join(dir, name), "utf8")))).join("\n");
}

test("should keep the page logged in across a restart", async ({ restartable, browser }) => {
    await restartable.start();
    const context = await browser.newContext({ baseURL: restartable.url, storageState: await restartable.api.storageState() });
    try {
        const page = await context.newPage();
        const connected = page.waitForResponse((response) => response.url().endsWith("/api/queue/"));
        await page.goto("/");
        await connected;
        await expect(page.getByTestId("logout")).toBeVisible();

        restartable.signal("SIGTERM");
        await restartable.waitForExit();
        // The page asks for the queue status each time its connection opens,
        // and only a valid session gets it
        const reconnected = page.waitForResponse((response) => response.url().endsWith("/api/queue/") && response.ok(), { timeout: 20000 });
        await restartable.start();
        await reconnected;
        await expect(page.getByTestId("logout")).toBeVisible();
        await expect(page).not.toHaveURL(/\/login/);
    } finally {
        await context.close();
    }
});

test("should let a running build finish and run a queued one after the restart", async ({ restartable }) => {
    await restartable.start();
    await expect(await restartable.api.post("/api/settings", { form: { concurrentBuilds: "1", buildHistorySize: "200" } })).toBeOK();
    const release = join(restartable.root, "release");
    const runs = join(restartable.root, "runs");
    const first = await restartable.runJob(await restartable.createJob(`tasks:\n  - name: wait\n    run: ${waitFor(release)}\n`));
    await restartable.waitForBuild(first, "running");
    const second = await restartable.runJob(await restartable.createJob(`tasks:\n  - name: count\n    run: echo $WAKE_BUILD_ID >> '${runs}'\n`));
    await restartable.waitForBuild(second, "pending");

    restartable.signal("SIGTERM");
    // wakeci keeps serving while it waits for the running build, and starts
    // no queued one
    await expect.poll(async () => (await (await restartable.api.get("/api/queue/")).json()).stopping).toBe(true);
    await restartable.waitForBuild(second, "pending");
    await writeFile(release, "");
    await restartable.waitForExit();

    await restartable.start();
    await restartable.waitForBuild(first, "finished");
    await restartable.waitForBuild(second, "finished");
    expect((await readFile(runs, "utf8")).trim().split("\n")).toEqual([String(second)]);
});

test("should abort a running build at the drain timeout and run its hooks", async ({ restartable }) => {
    await restartable.start(["draintimeout: 1s"]);
    const hook = join(restartable.root, "hook");
    const id = await restartable.runJob(
        await restartable.createJob(`tasks:\n  - name: wait\n    run: sleep 30\non_aborted:\n  - name: hook\n    run: touch '${hook}'\n`),
    );
    await restartable.waitForBuild(id, "running");

    restartable.signal("SIGTERM");
    await restartable.waitForExit();
    await access(hook);

    await restartable.start(["draintimeout: 1s"]);
    await restartable.waitForBuild(id, "aborted");
    expect(await taskLogs(restartable.root, id)).toContain("> Aborted: wakeci is stopping.");
});

test("should mark a build that ran when wakeci was killed as aborted", async ({ restartable }) => {
    await restartable.start();
    const release = join(restartable.root, "release");
    const id = await restartable.runJob(await restartable.createJob(`tasks:\n  - name: wait\n    run: ${waitFor(release)}\n`));
    await restartable.waitForBuild(id, "running");

    restartable.signal("SIGKILL");
    await restartable.waitForExit();
    // Ends the task, which outlives a killed wakeci
    await writeFile(release, "");

    await restartable.start();
    await restartable.waitForBuild(id, "aborted");
});
