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
    await wake.runJob(name);
    await wake.showFeed(name);
    const values = page.getByTestId("params-value");
    await expect(values).toHaveText(["ok0content_copyparam1", "ok1content_copyparam2", "ok2content_copyparam3"]);
    await page.getByTestId("expand-more-params-button").click();
    await expect(values).toHaveCount(4);
    await page.getByTestId("expand-less-params-button").click();
    await expect(values).toHaveCount(3);
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
