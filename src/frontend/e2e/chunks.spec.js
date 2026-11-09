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
