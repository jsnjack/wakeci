import { test, expect } from "./fixtures.js";

// The root's page runs its failed builds again, each with its values and
// under its parent
test("a root runs its failed builds again", async ({ page, wake, backend }) => {
    const child = await wake.createJob('params:\n  - N: ""\ntasks:\n  - name: check\n    run: test "$N" = 2\n');
    const root = await wake.createJob(`tasks:\n  - name: start\n    run: printf '1\\n2\\n3\\n' | wake start ${child} --each N --wait || true\n`);
    const id = await wake.runJob(root);
    await wake.waitForBuild(id);

    await wake.openBuild(id);
    const group = page.getByTestId("build-group");
    await group.getByTestId("group-run-failed").click();
    await expect(page.getByTestId("group-run-failed-list").locator("li")).toHaveText([new RegExp(`#\\d+ ${child} · 1$`)]);
    await page.getByTestId("group-run-failed-confirm").click();
    // --wait stopped at the first failure, so only N=1 ran and failed
    await expect(group.getByTestId("group-attempts")).toHaveText("2 attempts");
    const builds = (await (await backend.api.get(`/api/build/${id}/group`)).json()).builds;
    const again = builds.at(-1);
    expect([again.name, again.parent, Object.assign({}, ...again.params).N]).toEqual([child, id, "1"]);
});
