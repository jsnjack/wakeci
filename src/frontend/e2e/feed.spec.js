import { test, expect } from "./fixtures.js";
import { randomUUID } from "node:crypto";

test("should open Feed page", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("a[href='/']").filter({ hasText: "Feed" })).toContainText("Feed");
    await expect(page.locator("a[href='/jobs']")).toContainText("Jobs");
    await expect(page.locator("a[href='/settings']")).toContainText("Settings");
});

test("should filter builds", async ({ page, wake }) => {
    const name = await wake.createJob();
    await wake.showFeed(name);
    await expect(page.getByTestId("no-builds-found")).toContainText("No builds found");
    await page.getByTestId("filter").fill("");
    await wake.runJob(name);
    await page.getByTestId("filter").fill(name);
    await expect(page.getByTestId("feed-container").locator("[data-cy-build]")).toHaveCount(1);
    await expect(page.getByTestId("filtered-updates")).toBeHidden();
});

test("should hide updates when filter is active", async ({ page, wake }) => {
    const name = await wake.createJob();
    const otherName = await wake.createJob();
    await wake.showFeed(name);
    await wake.runJob(name);
    await expect(page.getByTestId("feed-container").locator("[data-cy-build]")).toHaveCount(1);
    await wake.runJob(otherName);
    await expect(page.getByTestId("filtered-updates")).toBeVisible();
    await expect(page.getByTestId("feed-container").locator("[data-cy-build]")).toHaveCount(1);
});

test("should keep showing new builds that match a search with prefixes", async ({ page, wake }) => {
    const value = `minsk-${randomUUID()}`;
    const name = await wake.createJob(`
desc: Prefix search
params:
  - CITY: ""
tasks:
  - name: Print env
    run: env
`);
    await wake.showFeed(`+name:${name} +CITY:${value}`);
    await wake.runJob(name, { CITY: value });
    await expect(page.getByTestId("feed-container").locator("[data-cy-build]")).toHaveCount(1);
    await expect(page.getByTestId("filtered-updates")).toBeHidden();
    // a build the search doesn't match is still left out
    await wake.runJob(name, { CITY: "elsewhere" });
    await expect(page.getByTestId("filtered-updates")).toBeVisible();
    await expect(page.getByTestId("feed-container").locator("[data-cy-build]")).toHaveCount(1);
});

test("should filter jobs with params", async ({ page, wake }) => {
    const value = `brest-${randomUUID()}`;
    const name = await wake.createJob(`
desc: Test env variables
params:
  - bereza: ${value}
tasks:
  - name: Print env
    run: env
`);
    await wake.runJob(name);
    await wake.showFeed(value);
    await expect(page.getByTestId("feed-container").locator("[data-cy-build]")).toHaveCount(1);
});

test("should show the params a build changed, five at most", async ({ page, wake }) => {
    const name = await wake.createJob(`
desc: Test env variables
params:
  - param1: ok0
  - param2: ok1
  - param3: ok2
  - param4: ok3
  - param5: ok4
  - param6: ok5
tasks:
  - name: Print env
    run: env
`);
    await wake.runJob(name, { param1: "new0", param2: "new1", param3: "new2", param4: "new3", param5: "new4", param6: "new5" });
    await wake.showFeed(name);
    await expect(page.getByTestId("params-value")).toHaveText(["new0", "new1", "new2", "new3", "new4"]);
    // The build page lists the rest
    await expect(page.locator("[data-cy-build]")).toContainText("+1");
});

test("should show only the params that differ from the job's defaults", async ({ page, wake }) => {
    const name = await wake.createJob(`
desc: Test env variables
params:
  - COMMIT: ""
  - BRANCH: main
  - SUITE: all
  - AUTHOR: nobody
tasks:
  - name: Print env
    run: env
`);
    await wake.runJob(name, { COMMIT: "abc123", AUTHOR: "alice" });
    await wake.showFeed(name);
    await expect(page.getByTestId("params-value")).toHaveText(["abc123", "alice"]);
    // The name and the full value show on hover
    await expect(page.getByTestId("params-value").first()).toHaveAttribute("data-tip", "COMMIT: abc123");
});

test("should show the labels first, and a commit hash short", async ({ page, wake }) => {
    const commit = "3f9c2a1e5d4b8a0c7f6e2d1b9a8c7f6e5d4b3a2c";
    const name = await wake.createJob(`
desc: Labels
labels: [BRANCH, AUTHOR]
params:
  - COMMIT: ""
  - AUTHOR: unknown
  - BRANCH: main
tasks:
  - name: Print env
    run: env
`);
    await wake.runJob(name, { COMMIT: commit, AUTHOR: "alice", BRANCH: "T-4821-fix-login" });
    await wake.showFeed(name);
    await expect(page.getByTestId("params-value")).toHaveText(["T-4821-fix-login", "alice", "3f9c2a1"]);
    // A label's value searches by its name
    await page.getByTestId("params-value").first().click();
    await expect(page.getByTestId("filter")).toHaveValue("+branch=T-4821-fix-login");
});

test("should say where a queued build waits", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
desc: Queue place
tasks:
  - name: Wait
    run: ${gate.command}
concurrency: 1
`);
    await wake.runJob(name);
    const queued = await wake.runJob(name);
    await wake.showFeed(name);
    const row = page.locator(`[data-cy-build="${queued}"]`);
    await expect(row.getByTestId("queue-place")).toHaveText(/^\d+(st|nd|rd|th) in queue$/);
    await expect(row.getByTestId("started-ago")).toHaveText(/^waiting \d+s$/);
    await gate.release();
});

test("should run a build again with its params", async ({ page, wake }) => {
    const name = await wake.createJob(`
desc: Run again
params:
  - TARGET: staging
tasks:
  - name: Print env
    run: env
`);
    const first = await wake.runJob(name, { TARGET: "prod" });
    await wake.waitForBuild(first);
    await wake.showFeed(name);
    const builds = page.locator("[data-cy-build]");
    await expect(builds).toHaveCount(1);
    await builds.first().getByTestId("run-again-button").click();
    await expect(page.locator(".notification-content")).toContainText(`${name} has been scheduled`);
    await expect(builds).toHaveCount(2);
    await expect(builds.first().getByTestId("params-value")).toHaveText(["prod"]);
    // A finished build can't be aborted, nor a running one run again
    await expect(builds.last().getByTestId("abort-build-button")).toHaveCount(0);
});

test("should open a build from anywhere on its row", async ({ page, wake }) => {
    const name = await wake.createJob();
    const id = await wake.runJob(name);
    await wake.showFeed(name);
    await page.locator(`[data-cy-build="${id}"] [data-cy=duration]`).click();
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
});

test("should preserve filter after reload", async ({ page, wake }) => {
    const name = await wake.createJob(undefined, `job with spaces ${Date.now()}`);
    const id = await wake.runJob(name);
    const filter = `"${name}"`;
    await wake.showFeed(filter);
    await expect(page.getByTestId("open-build-button")).toHaveCount(1);
    await page.getByTestId("open-build-button").click();
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.goBack();
    await expect(page.getByTestId("open-build-button")).toHaveCount(1);
    await expect(page.getByTestId("filter")).toHaveValue(filter);
    await page.reload();
    await expect(page.getByTestId("filter")).toHaveValue(filter);
    await expect(page.getByTestId("open-build-button")).toHaveCount(1);
});

test("should start job immediately", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
desc: Queue bypass test
tasks:
  - name: Wait for test
    run: ${gate.command}
concurrency: 1
`);
    const first = await wake.runJob(name);
    const second = await wake.runJob(name);
    await wake.showFeed(name);
    const firstStatus = page.locator(`[data-cy-build="${first}"] [data-cy-status]`);
    const secondStatus = page.locator(`[data-cy-build="${second}"] [data-cy-status]`);
    await expect(firstStatus).toHaveAttribute("data-cy-status", "running");
    await expect(secondStatus).toHaveAttribute("data-cy-status", "pending");
    await page.locator(`[data-cy-build="${second}"] [data-cy=start-build-button]:not([disabled])`).click();
    await expect(secondStatus).toHaveAttribute("data-cy-status", "running");
    await expect(firstStatus).toHaveAttribute("data-cy-status", "running");
    await gate.release();
    await wake.waitForBuild(first);
    await wake.waitForBuild(second);
});

test("should keep the filter in the address bar", async ({ page }) => {
    await page.goto("/");
    await page.getByTestId("filter").fill("status:failed env:a b");
    await expect(page).toHaveURL(/\?q=/);
    await page.reload();
    await expect(page.getByTestId("filter")).toHaveValue("status:failed env:a b");
    await page.getByTestId("filter").fill("");
    await expect(page).not.toHaveURL(/[?&](q|filter)=/);
});

test("should filter by a build's parameter when it is clicked", async ({ page, wake }) => {
    // Short enough to be shown in full
    const target = `prod-${randomUUID().slice(0, 8)}`;
    const name = await wake.createJob(`desc: Params\nparams:\n  - TARGET: staging\ntasks:\n  - name: hello\n    run: echo hello\n`);
    await wake.showFeed(name);
    // The feed leaves out params at their default
    await wake.runJob(name, { TARGET: `${target}-fix` });
    await wake.runJob(name, { TARGET: target });
    const builds = page.getByTestId("feed-container").locator("[data-cy-build]");
    await expect(builds).toHaveCount(2);

    // The search finds the value exactly, so not the build with -fix
    await builds.getByTestId("params-value").getByText(target, { exact: true }).click();
    await expect(page.getByTestId("filter")).toHaveValue(`+target=${target}`);
    await expect(builds).toHaveCount(1);
    // Ctrl+click adds the parameter once
    await builds
        .getByTestId("params-value")
        .getByText(target, { exact: true })
        .click({ modifiers: ["Control"] });
    await expect(page.getByTestId("filter")).toHaveValue(`+target=${target}`);
});

test("should show the filter in the address bar encoded once", async ({ page }) => {
    await page.goto("/");
    await page.getByTestId("filter").fill("status:failed env:a b");
    await expect(page).toHaveURL(/\?q=status:failed\+env:a\+b$/);
    await page.reload();
    await expect(page.getByTestId("filter")).toHaveValue("status:failed env:a b");
});

test("should keep a filter with escapes and percent signs as typed", async ({ page }) => {
    const filter = "TARGET:a%2Fb name:100%25 50%";
    await page.goto("/");
    await page.getByTestId("filter").fill(filter);
    await expect(page).toHaveURL(/\?q=/);
    await page.reload();
    await expect(page.getByTestId("filter")).toHaveValue(filter);
});

test("should open a feed link with the filter encoded twice, as wakeci wrote it before", async ({ page }) => {
    await page.goto("/?filter=status%253Afailed%2520env%253Aa%2520b");
    await expect(page.getByTestId("filter")).toHaveValue("status:failed env:a b");
    // The address bar moves to the new parameter
    await expect(page).toHaveURL(/\/\?q=status:failed\+env:a\+b$/);
});

test("should open an old feed link with a percent sign", async ({ page }) => {
    // What wakeci wrote before for name:100%
    await page.goto("/?filter=name%253A100%2525");
    await expect(page.getByTestId("filter")).toHaveValue("name:100%");
});

test("should take q over an old filter parameter", async ({ page }) => {
    await page.goto("/?q=env:new&filter=env%253Aold");
    await expect(page.getByTestId("filter")).toHaveValue("env:new");
    await expect(page).toHaveURL(/\/\?q=env:new$/);
});

test("should load older builds with before", async ({ page, wake }) => {
    const name = await wake.createJob(`
desc: Paging
tasks:
  - name: Nothing
    run: "true"
`);
    // One more than the first page, which has 15
    const ids = [];
    for (let i = 0; i < 16; i++) {
        ids.push(await wake.runJob(name));
    }
    await wake.showFeed(name);
    const builds = page.getByTestId("feed-container").locator("[data-cy-build]");
    await expect(builds).toHaveCount(15);
    const more = page.waitForRequest((request) => new URL(request.url()).pathname === "/api/feed" && request.url().includes("before="));
    await page.getByTestId("more").click();
    expect(new URL((await more).url()).searchParams.get("before")).toBe(String(ids[1]));
    await expect(builds).toHaveCount(16);
    await expect(page.locator(`[data-cy-build="${ids[0]}"]`)).toBeVisible();
    await expect(page.getByTestId("more")).toBeHidden();
});

// F9: on a wide window a build opens beside the feed, which keeps its search
test("should show a build beside the feed on a wide window", async ({ page, wake }) => {
    await page.setViewportSize({ width: 1600, height: 900 });
    const name = await wake.createJob();
    const first = await wake.runJob(name);
    const second = await wake.runJob(name);
    await wake.waitForBuild(first);
    await wake.waitForBuild(second);
    await wake.showFeed(name);
    await page.locator(`[data-cy-build="${second}"]`).getByTestId("open-build-button").click();
    await expect(page).toHaveURL(new RegExp(`[?&]build=${second}(&|$)`));
    await expect(page.getByTestId("build-pane")).toContainText(`#${second}`);
    await expect(page.getByTestId("page-title")).toHaveText("Feed");
    await expect(page.locator(`[data-cy-build="${second}"]`)).toHaveAttribute("aria-current", "true");
    // Down moves to the next, older build
    await page.keyboard.press("ArrowDown");
    await expect(page).toHaveURL(new RegExp(`[?&]build=${first}(&|$)`));
    // The address names both, so a reload shows them again
    await page.reload();
    await expect(page.getByTestId("build-pane")).toContainText(`#${first}`);
    await expect(page.getByTestId("filter")).toHaveValue(name);
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("build-pane")).toHaveCount(0);
    await expect(page).not.toHaveURL(/build=/);
});

test("should keep the build beside the feed in sight while the feed scrolls", async ({ page, wake }) => {
    await page.setViewportSize({ width: 1600, height: 500 });
    const name = await wake.createJob();
    let newest = 0;
    for (let i = 0; i < 12; i++) {
        newest = await wake.runJob(name);
        await wake.waitForBuild(newest);
    }
    await wake.showFeed(name);
    await page.locator(`[data-cy-build="${newest}"]`).getByTestId("open-build-button").click();
    await expect(page.getByTestId("build-pane")).toContainText(`#${newest}`);
    await page.evaluate(
        () =>
            new Promise((resolve) => {
                window.addEventListener("scroll", resolve, { once: true });
                window.scrollTo(0, document.body.scrollHeight);
            }),
    );
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(200);
    await expect(page.getByTestId("build-pane")).toBeInViewport({ ratio: 0.9 });
});

test("should open a shared feed address with a build as the build's page on a narrow window", async ({ page, wake }) => {
    const name = await wake.createJob();
    const id = await wake.runJob(name);
    await page.goto(`/?build=${id}`);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
});
