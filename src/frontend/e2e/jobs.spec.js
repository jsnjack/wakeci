import { randomUUID } from "node:crypto";
import { test, expect } from "./fixtures.js";

async function createJob(page, name) {
    await page.getByTestId("create-job").click();
    await page.locator("input[name=new-job-name]").fill(name);
    await page.getByTestId("create-job-button").click();
    await expect(page.locator(".notification-content")).toContainText("New job created");
}

function row(page, name) {
    return page
        .locator(".row")
        .filter({ has: page.locator("[data-cy=run-job-button]") })
        .filter({ hasText: name });
}

async function deleteJob(page, name) {
    const job = page.getByTestId(name);
    await job.getByTestId("delete-job-button").click();
    const refreshed = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/jobs/" && response.request().method() === "GET");
    await job.getByTestId("delete-job-confirm").click();
    const response = await refreshed;
    expect(response.status()).toBe(200);
    expect(((await response.json()) || []).map((item) => item.name)).not.toContain(name);
    await expect(job).toHaveCount(0);
}

test("should handle API network errors without crashing", async ({ page }) => {
    await page.route("**/api/jobs/", (route) => route.abort());
    await page.goto("/jobs");
    await expect(page).toHaveURL(/\/jobs$/);
    await expect(page.locator(".notification-content")).toContainText("Network Error");
    await expect(page.getByTestId("jobs-container")).toContainText("No jobs found");
});

test("should create a new job", async ({ page, wake }) => {
    const name = wake.trackJob();
    await page.goto("/jobs");
    await createJob(page, name);
    await expect(page.getByTestId(name)).toBeVisible();
    await page.getByTestId("create-job").click();
    await page.locator("input[name=new-job-name]").fill(name);
    await page.getByTestId("create-job-button").click();
    await expect(page.locator(".notification-content").last()).toContainText("Job with this name already exists");
});

test("should delete a job", async ({ page }) => {
    const name = `delete-job-${Date.now()}`;
    await page.goto("/jobs");
    await createJob(page, name);
    await expect(page.getByTestId(name)).toBeVisible();
    await deleteJob(page, name);
});

test("should escape HTML in a job name shown in notifications", async ({ page, wake }) => {
    const prefix = `job-xss-${Date.now()}`;
    const name = wake.trackJob(`${prefix}<img src=x onerror="window.__xssTriggered = true">`);
    await page.goto("/jobs");
    await createJob(page, name);
    await row(page, prefix).getByTestId("run-job-button").click();
    await row(page, prefix).getByTestId("start-job-confirm").click();
    const notification = page.locator(".notification-content").filter({ hasText: "has been scheduled" });
    await expect(notification).toContainText(`${name} has been scheduled`);
    await expect(notification.locator("img")).toHaveCount(0);
    expect(await page.evaluate(() => window.__xssTriggered)).toBeUndefined();
});

test("should handle reserved URL characters in a job name", async ({ page, wake }) => {
    const name = await wake.createJob(undefined, `job#?% ${Date.now()}`);
    await page.goto("/jobs");
    await row(page, name).getByTestId("run-job-button").click();
    await row(page, name).getByTestId("start-job-confirm").click();
    await expect(page.locator(".notification-content")).toContainText(`${name} has been scheduled`);
    const build = page.locator(".notification-content a[href^='/build/']");
    await expect(build).toHaveAttribute("href", /^\/build\/\d+\/$/);
    await build.click();
    await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
    await expect(page.locator("[data-cy-status]").first()).toHaveAttribute("data-cy-status", "finished");
    await page.goto("/jobs");
    await row(page, name).getByTestId("edit-job-button").click();
    await expect(page).toHaveURL(`${wake.url}/job/${encodeURIComponent(name)}`);
    await expect(page.locator("[data-cy=editor] .cm-content")).toContainText("desc: New job");
    await page.getByTestId("save-button").click();
    await expect(page.locator(".notification-content").last()).toContainText("Saved");
    await page.goto("/jobs");
    await deleteJob(page, name);
});

test("should edit a job", async ({ page, wake }) => {
    const name = wake.trackJob();
    await page.goto("/jobs");
    await createJob(page, name);
    await page.getByTestId(name).getByTestId("edit-job-button").click();
    await expect(page).toHaveURL(`${wake.url}/job/${name}`);
    await page.locator("[data-cy=editor] .cm-content").fill("desc: Empty job");
    await page.getByTestId("save-button").click();
    await expect(page.locator(".notification-content").last()).toContainText("Saved");
    await page.goto("/jobs");
    await expect(page.getByTestId(name)).toContainText("Empty job");
});

test("should start a job", async ({ page, wake }) => {
    const name = wake.trackJob();
    await page.goto("/jobs");
    await createJob(page, name);
    await page.getByTestId(name).getByTestId("run-job-button").click();
    await page.getByTestId(name).getByTestId("start-job-confirm").click();
    await expect(page.locator(".notification-content").last()).toContainText(`${name} has been scheduled`);
    await page.goto("/");
    await expect(page.locator("body")).toContainText(name);
});

test("should edit a job and navigate", async ({ page, wake }) => {
    const name = wake.trackJob();
    await page.goto("/jobs");
    await createJob(page, name);
    await page.getByTestId(name).getByTestId("edit-job-button").click();
    await expect(page).toHaveURL(`${wake.url}/job/${name}`);
    await page.getByTestId("save-and-close-button").click();
    await expect(page).toHaveURL(`${wake.url}/jobs`);
});

test("should filter jobs", async ({ page, wake }) => {
    const name = await wake.createJob();
    await page.goto("/jobs");
    await page.getByTestId("filter").fill(`${name}bad`);
    await expect(page.getByTestId("jobs-container")).toContainText("No jobs found");
    await page.getByTestId("filter").fill(name);
    await expect(page.getByTestId("jobs-container")).toHaveCount(1);
    await expect(page.getByTestId(name)).toBeVisible();
});

test("should open the editor of a job whose name has a dot by its URL", async ({ page, wake }) => {
    const name = await wake.createJob("desc: Dotted\ntasks:\n  - name: hello\n    run: echo hello\n", `job-${randomUUID()}.prod`);
    await page.goto(`/job/${name}`);
    await expect(page.locator("[data-cy=editor] .cm-content")).toContainText("desc: Dotted");
});
