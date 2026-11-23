import { test, expect } from "./fixtures.js";

// docs/wake.md: a task starts builds with the wake command and its build's
// token; they are shown under its build, and the token never shows in a log
test("a task starts builds with wake, under its build", async ({ page, wake, backend }) => {
    const child = await wake.createJob('params:\n  - NODE: ""\n  - REGION: us\ntasks:\n  - name: work\n    run: echo "node=$NODE region=$REGION"\n');
    const parent = await wake.createJob(
        "params:\n  - REGION: eu\n" +
            "tasks:\n" +
            '  - name: token\n    run: echo "token=$WAKE_TOKEN"\n' +
            "  - name: who\n    run: wake api /api/whoami\n" +
            `  - name: each\n    run: printf 'a\\nb\\n' | wake start ${child} --inherit --each NODE\n` +
            `  - name: wait\n    run: wake start ${child} NODE=c --wait\n` +
            `  - name: typo\n    run: wake start ${child} no-such-job\n    ignore_errors: true\n`,
    );
    const id = await wake.runJob(parent);
    await wake.waitForBuild(id);

    const log = async (task) => await (await backend.api.get(`/api/build/${id}/log/${task}`)).text();
    const token = await log(0);
    expect(token).toContain("token=***REDACTED***");
    expect(token).not.toContain("wb_");
    expect(await log(1)).toContain(`"auth":"build","user":"","admin":false,"build":${id},`);
    expect(await log(2)).toMatch(new RegExp(`Started ${child} #\\d+: http://\\S+/build/\\d+[\\s\\S]*Started ${child} #\\d+`));
    expect(await log(3)).toMatch(new RegExp(`${child} #\\d+: finished`));
    const typo = await log(4);
    expect(typo).toContain("There is no job no-such-job.");
    expect(typo).toContain("Nothing was started.");

    const group = await (await backend.api.get(`/api/build/${id}/group`)).json();
    const children = group.builds.filter((build) => build.id !== id);
    expect(children.map((build) => [build.name, build.parent])).toEqual([
        [child, id],
        [child, id],
        [child, id],
    ]);
    const values = children.map((build) => Object.assign({}, ...build.params));
    // --inherit passed REGION on; the last one wasn't asked to
    expect(values.map((v) => `${v.NODE} ${v.REGION}`)).toEqual(["a eu", "b eu", "c us"]);

    await wake.openBuild(id);
    await expect(page.getByTestId("build-group").getByRole("heading")).toHaveText("Started 3 builds");
});
