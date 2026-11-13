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

test("should toggle params", async ({ page, wake }) => {
    const name = await wake.createJob(`
desc: Test env variables
params:
  - param1: ok0
  - param2: ok1
  - param3: ok2
  - param4: ok3
tasks:
  - name: Print env
    run: env
`);
    await wake.runJob(name, { param1: "new0", param2: "new1", param3: "new2", param4: "new3" });
    await wake.showFeed(name);
    const values = page.getByTestId("params-value");
    await expect(values).toHaveText(["new0content_copyparam1", "new1content_copyparam2", "new2content_copyparam3"]);
    await page.getByTestId("expand-more-params-button").click();
    await expect(values).toHaveCount(4);
    await page.getByTestId("expand-less-params-button").click();
    await expect(values).toHaveCount(3);
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
    const values = page.getByTestId("params-value");
    await expect(values).toHaveText(["abc123content_copyCOMMIT", "alicecontent_copyAUTHOR"]);
    await page.getByTestId("expand-more-params-button").click();
    await expect(values).toHaveText(["abc123content_copyCOMMIT", "maincontent_copyBRANCH", "allcontent_copySUITE", "alicecontent_copyAUTHOR"]);
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
    await wake.runJob(name, { TARGET: target });
    const builds = page.getByTestId("feed-container").locator("[data-cy-build]");
    await expect(builds).toHaveCount(1);

    await builds.getByTestId("params-value").getByText(target).click();
    await expect(page.getByTestId("filter")).toHaveValue(`+"TARGET:${target}"`);
    await expect(builds).toHaveCount(1);
    // Ctrl+click adds the parameter once
    await builds
        .getByTestId("params-value")
        .getByText(target)
        .click({ modifiers: ["Control"] });
    await expect(page.getByTestId("filter")).toHaveValue(`+"TARGET:${target}"`);
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
