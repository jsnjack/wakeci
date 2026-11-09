import { test, expect } from "./fixtures.js";

// scriptBytes returns how many bytes of JavaScript the page has loaded so far
function scriptBytes(page) {
    return page.evaluate(() =>
        performance
            .getEntriesByType("resource")
            .filter((entry) => new URL(entry.name).pathname.endsWith(".js"))
            .reduce((sum, entry) => sum + entry.decodedBodySize, 0),
    );
}

test("should load the job editor's code only when the editor opens", async ({ page, wake }) => {
    const name = await wake.createJob("desc: Lazy\ntasks:\n  - name: hello\n    run: echo hello\n");
    await page.goto("/");
    await expect(page.getByTestId("filter")).toBeVisible();
    const feed = await scriptBytes(page);
    expect(feed).toBeGreaterThan(0);
    // Vite's chunk size limit; the editor alone is close to it
    expect(feed).toBeLessThan(500 * 1000);

    await page.goto(`/job/${name}`);
    await expect(page.locator("[data-cy=editor] .cm-content")).toContainText("desc: Lazy");
    // The editor's own chunk came with it
    expect(await scriptBytes(page)).toBeGreaterThan(feed);
});

// failJobsChunk makes the Jobs page's chunk answer 404, as a chunk only the
// build before a self-update had does, the first `times` times, and returns
// how often it was asked for
async function failJobsChunk(page, times) {
    const asked = { count: 0 };
    await page.route(/\/assets\/JobsView-[\w-]+\.js$/, (route) => {
        asked.count++;
        return asked.count <= times ? route.fulfill({ status: 404, body: "" }) : route.continue();
    });
    return asked;
}

test("should load a page in full when its code fails to load", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("filter")).toBeVisible();
    const asked = await failJobsChunk(page, 1);
    let loads = 0;
    page.on("load", () => loads++);

    await page.locator('nav a[href="/jobs"]').click();
    await expect(page).toHaveURL(/\/jobs$/);
    await expect(page.getByTestId("jobs-container")).toBeVisible();
    expect(asked.count).toBeGreaterThan(1);
    expect(loads).toBe(1);
});

test("should load a page in full only once when its code keeps failing", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("filter")).toBeVisible();
    const asked = await failJobsChunk(page, Infinity);
    let loads = 0;
    page.on("load", () => loads++);

    await page.locator('nav a[href="/jobs"]').click();
    await expect.poll(() => loads).toBe(1);
    await expect.poll(() => asked.count).toBeGreaterThan(1);
    // Gives a second reload, if any, time to start
    await page.waitForTimeout(2000);
    expect(loads).toBe(1);
});
