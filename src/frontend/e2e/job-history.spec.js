import { test, expect } from "./fixtures.js";

// B7: the job's last builds next to the title, and keys to the ones around
test("the build page shows the job's last builds and moves between them", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    const ids = [];
    for (let i = 0; i < 3; i++) {
        const id = await wake.runJob(name);
        await wake.waitForBuild(id);
        ids.push(id);
    }
    const [first, middle, last] = ids;
    await wake.openBuild(middle);
    const strip = page.getByTestId("job-history").getByTestId("outcome-strip");
    await expect(strip.locator("[data-cy-status]")).toHaveCount(3);
    await expect(strip.getByTestId("strip-current")).toHaveCount(1);

    await page.keyboard.press(",");
    await expect(page).toHaveURL(`/build/${first}`);
    await expect(page.getByTestId("page-title")).toContainText(`#${first}`);
    // Nothing older
    await page.keyboard.press(",");
    await expect(page).toHaveURL(`/build/${first}`);
    await page.keyboard.press(".");
    await expect(page).toHaveURL(`/build/${middle}`);
    await page.keyboard.press(".");
    await expect(page).toHaveURL(`/build/${last}`);
    // The same with the buttons
    const history = page.getByTestId("job-history");
    await expect(history.getByTestId("job-newer")).toBeDisabled();
    await history.getByTestId("job-older").click();
    await expect(page).toHaveURL(`/build/${middle}`);
});

// The feed answers newest first: with more newer builds than one request
// reads, "." still goes to the build right after this one
test("the next build is the one right after, however many are newer", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    const id = await wake.runJob(name);
    await wake.waitForBuild(id);
    // 300 newer builds of the job, this one not among its last 20
    const fake = (n) => ({ id: n, name, status: "finished", params: [], startedAt: new Date().toISOString(), duration: 1 });
    await page.route(
        (url) => url.pathname === "/api/feed" && url.searchParams.get("job") === name,
        (route) => {
            const query = new URL(route.request().url()).searchParams;
            const after = Number(query.get("after") || 0);
            const before = Number(query.get("before") || id + 301);
            const limit = Number(query.get("limit"));
            if (!after && !query.get("before")) {
                return route.fulfill({ json: [...Array(limit)].map((_, i) => fake(id + 300 - i)) });
            }
            if (!after) {
                return route.continue();
            }
            const ids = [];
            for (let n = before - 1; n > after && ids.length < limit; n--) {
                ids.push(n);
            }
            return route.fulfill({ json: ids.map(fake) });
        },
    );
    await wake.openBuild(id);
    await expect(page.getByTestId("job-history").getByTestId("job-newer")).toBeEnabled();
    await page.keyboard.press(".");
    await expect(page).toHaveURL(`/build/${id + 1}`);
});
