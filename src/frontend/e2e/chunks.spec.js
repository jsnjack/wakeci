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

// failJobsChunk makes the Jobs page's chunk answer 404, after delay, the first
// `times` times, as a chunk that only the build before an update had, and
// returns how often it was asked for
async function failJobsChunk(page, times, delay = 0) {
    const asked = { count: 0 };
    await page.route(/\/assets\/JobsView-[\w-]+\.js$/, async (route) => {
        asked.count++;
        if (asked.count > times) {
            return route.continue();
        }
        if (delay > 0) {
            await new Promise((done) => setTimeout(done, delay));
        }
        return route.fulfill({ status: 404, body: "" });
    });
    return asked;
}

// updateServer makes /api/queue/ report another wakeci version from now on, as
// after a self-update; delay holds each answer that long
async function updateServer(page, delay = 0) {
    await page.route("**/api/queue/", async (route) => {
        const response = await route.fetch();
        const status = await response.json();
        if (delay > 0) {
            await new Promise((done) => setTimeout(done, delay));
        }
        await route.fulfill({ response, json: { ...status, version: "99.0.0" } });
    });
}

// openFeed opens the feed once the page knows the version it was loaded
// from, and counts the page's full loads from then on
async function openFeed(page) {
    const versionKnown = page.waitForResponse((response) => response.url().endsWith("/api/queue/"));
    await page.goto("/");
    await versionKnown;
    await expect(page.getByTestId("filter")).toBeVisible();
    const counter = { loads: 0 };
    page.on("load", () => counter.loads++);
    return counter;
}

test("should load a page in full when its code fails to load after an update", async ({ page }) => {
    const counter = await openFeed(page);
    await failJobsChunk(page, 1);
    await updateServer(page);

    await page.locator('nav a[href="/jobs"]').click();
    await expect(page.getByTestId("jobs-container")).toBeVisible();
    await expect(page).toHaveURL(/\/jobs$/);
    // The load event may reach the test after the page shows
    await expect.poll(() => counter.loads).toBe(1);
});

test("should not load a page in full when wakeci runs the same version", async ({ page }) => {
    const errors = [];
    page.on("console", (message) => message.type() === "error" && errors.push(message.text()));
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity);

    await page.locator('nav a[href="/jobs"]').click();
    await expect(page.locator(".notification-content")).toContainText("couldn't be loaded");
    expect(counter.loads).toBe(0);
    // Registering router.onError stops vue-router from logging it
    expect(errors.some((text) => text.includes("Navigation to /jobs failed"))).toBe(true);
});

test("should load a page in full only once after an update when its code keeps failing", async ({ page }) => {
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity);
    await updateServer(page);

    await page.locator('nav a[href="/jobs"]').click();
    await expect.poll(() => counter.loads).toBe(1);
    // Shown once the loaded page decided not to load again
    await expect(page.locator(".notification-content")).toContainText("couldn't be loaded");
    expect(counter.loads).toBe(1);
});

test("should not load a page in full while wakeci doesn't answer", async ({ page }) => {
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity);
    // As while wakeci restarts
    await page.route("**/api/queue/", (route) => route.abort("connectionrefused"));

    await page.locator('nav a[href="/jobs"]').click();
    await expect(page.locator(".notification-content")).toContainText("couldn't be loaded");
    expect(counter.loads).toBe(0);
    await expect(page.getByTestId("filter")).toBeVisible();
});

test("should not load a page in full when another page was opened meanwhile", async ({ page }) => {
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity);
    await updateServer(page, 1500);
    const answered = page.waitForResponse((response) => response.url().endsWith("/api/queue/"));

    await page.locator('nav a[href="/jobs"]').click();
    await page.locator('nav a[href="/settings"]').click();
    await expect(page.getByTestId("version")).toBeVisible();
    await answered;
    // Lets the page act on the answer
    await page.evaluate(() => new Promise((done) => setTimeout(done, 100)));
    await expect(page).toHaveURL(/\/settings$/);
    expect(counter.loads).toBe(0);
});

test("should not load a page in full when its chunk fails after another page opened", async ({ page }) => {
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity, 1500);
    await updateServer(page);
    const answered = page.waitForResponse((response) => response.url().endsWith("/api/queue/"));

    await page.locator('nav a[href="/jobs"]').click();
    await page.locator('nav a[href="/settings"]').click();
    await expect(page.getByTestId("version")).toBeVisible();
    await answered;
    // Lets the page act on the answer
    await page.evaluate(() => new Promise((done) => setTimeout(done, 100)));
    await expect(page).toHaveURL(/\/settings$/);
    expect(counter.loads).toBe(0);
});

test("should say a page couldn't be loaded when wakeci doesn't answer in time", async ({ page }) => {
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity);
    // Accepts the request and never answers
    await page.route("**/api/queue/", () => {});

    await page.locator('nav a[href="/jobs"]').click();
    await expect(page.locator(".notification-content")).toContainText("couldn't be loaded", { timeout: 15000 });
    expect(counter.loads).toBe(0);
});

test("should not load a page in full when the shown page was opened again meanwhile", async ({ page }) => {
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity);
    await updateServer(page, 1500);
    const answered = page.waitForResponse((response) => response.url().endsWith("/api/queue/"));

    await page.locator('nav a[href="/jobs"]').click();
    // The feed is still shown while Jobs loads, so vue-router skips beforeEach
    await page.locator('nav a.button[href="/"]').click();
    await answered;
    // Lets the page act on the answer
    await page.evaluate(() => new Promise((done) => setTimeout(done, 100)));
    await expect(page).toHaveURL(/:\d+\/$/);
    expect(counter.loads).toBe(0);
});

test("should load a page in full after an update when the feed answers meanwhile", async ({ page }) => {
    let releaseFeed;
    const feedHeld = new Promise((done) => (releaseFeed = done));
    await page.route(/\/api\/feed\?/, async (route) => {
        await feedHeld;
        await route.continue();
    });
    const counter = await openFeed(page);
    await failJobsChunk(page, Infinity);
    await updateServer(page, 1500);
    const checking = page.waitForRequest((request) => request.url().endsWith("/api/queue/"));

    await page.locator('nav a[href="/jobs"]').click();
    await checking;
    // The feed puts its filter in the URL when it answers, the same URL here
    const feedAnswered = page.waitForResponse(/\/api\/feed\?/);
    releaseFeed();
    await feedAnswered;
    await expect.poll(() => counter.loads).toBe(1);
    await expect(page).toHaveURL(/\/jobs$/);
});

test("should open a page whose code loads while the feed answers", async ({ page }) => {
    let releaseFeed;
    const feedHeld = new Promise((done) => (releaseFeed = done));
    await page.route(/\/api\/feed\?/, async (route) => {
        await feedHeld;
        await route.continue();
    });
    await openFeed(page);
    let releaseJobs;
    const jobsHeld = new Promise((done) => (releaseJobs = done));
    const jobsAsked = page.waitForRequest(/\/assets\/JobsView-[\w-]+\.js$/);
    await page.route(/\/assets\/JobsView-[\w-]+\.js$/, async (route) => {
        await jobsHeld;
        await route.continue();
    });

    await page.locator('nav a[href="/jobs"]').click();
    await jobsAsked;
    const feedAnswered = page.waitForResponse(/\/api\/feed\?/);
    releaseFeed();
    await feedAnswered;
    // Lets the feed act on the answer before the page's code arrives
    await page.evaluate(() => new Promise((done) => setTimeout(done, 100)));
    releaseJobs();
    await expect(page.getByTestId("jobs-container")).toBeVisible();
    await expect(page).toHaveURL(/\/jobs$/);
});

test("should open a page whose code loads after a filter was typed", async ({ page }) => {
    let releaseFeed;
    const feedHeld = new Promise((done) => (releaseFeed = done));
    await page.route(/\/api\/feed\?/, async (route) => {
        await feedHeld;
        await route.continue();
    });
    await openFeed(page);
    let releaseJobs;
    const jobsHeld = new Promise((done) => (releaseJobs = done));
    const jobsAsked = page.waitForRequest(/\/assets\/JobsView-[\w-]+\.js$/);
    await page.route(/\/assets\/JobsView-[\w-]+\.js$/, async (route) => {
        await jobsHeld;
        await route.continue();
    });

    await page.getByTestId("filter").fill("abc");
    await page.locator('nav a[href="/jobs"]').click();
    await jobsAsked;
    const filteredFeedAnswered = page.waitForResponse(/\/api\/feed\?.*q=abc/);
    releaseFeed();
    await filteredFeedAnswered;
    // Lets the feed act on the answer before the page's code arrives
    await page.evaluate(() => new Promise((done) => setTimeout(done, 100)));
    releaseJobs();
    await expect(page.getByTestId("jobs-container")).toBeVisible();
    await expect(page).toHaveURL(/\/jobs$/);
});
