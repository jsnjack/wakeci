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
    // The single chunk was 876 kB, the editor's code alone is about 430 kB
    expect(feed).toBeLessThan(500 * 1000);

    // In the app, so the browser keeps counting what it loaded
    await page.locator('nav a[href="/jobs"]').click();
    await page.getByTestId("filter").fill(name);
    await expect(page.getByTestId("edit-job-button")).toHaveCount(1);
    await page.getByTestId("edit-job-button").click();
    await expect(page.locator("[data-cy=editor] .cm-content")).toContainText("desc: Lazy");
    expect((await scriptBytes(page)) - feed).toBeGreaterThan(300 * 1000);
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
    const errors = [];
    page.on("console", (message) => message.type() === "error" && errors.push(message.text()));
    await page.goto("/");
    await expect(page.getByTestId("filter")).toBeVisible();
    const asked = await failJobsChunk(page, Infinity);
    let loads = 0;
    page.on("load", () => loads++);

    await page.locator('nav a[href="/jobs"]').click();
    await expect.poll(() => loads).toBe(1);
    await expect.poll(() => asked.count).toBeGreaterThan(1);
    await expect(page.locator(".notification-content")).toContainText("couldn't be loaded");
    // Gives a second reload, if any, time to start
    await page.waitForTimeout(2000);
    expect(loads).toBe(1);
    // Registering router.onError stops vue-router from logging it
    expect(errors.some((text) => text.includes("Navigation to /jobs failed"))).toBe(true);
});

test("should not load a page in full while wakeci doesn't answer", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("filter")).toBeVisible();
    await failJobsChunk(page, Infinity);
    // As while wakeci restarts
    await page.route(
        (url) => url.pathname === "/",
        (route) => route.abort("connectionrefused"),
    );
    let loads = 0;
    page.on("load", () => loads++);

    await page.locator('nav a[href="/jobs"]').click();
    await expect(page.locator(".notification-content")).toContainText("couldn't be loaded");
    expect(loads).toBe(0);
    await expect(page.getByTestId("filter")).toBeVisible();
});

test("should load a page in full again once an earlier reload mark is old", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("filter")).toBeVisible();
    // Left by a reload for /jobs a minute ago that ended on another page
    await page.evaluate(() => sessionStorage.setItem("wakeci-chunk-reload", JSON.stringify({ path: "/jobs", at: Date.now() - 60 * 1000 })));
    await failJobsChunk(page, 1);
    let loads = 0;
    page.on("load", () => loads++);

    await page.locator('nav a[href="/jobs"]').click();
    await expect(page.getByTestId("jobs-container")).toBeVisible();
    expect(loads).toBe(1);
});
