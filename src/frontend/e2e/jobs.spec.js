import { randomUUID } from "node:crypto";
import { test, expect } from "./fixtures.js";

async function createJob(page, name) {
    await page.getByTestId("create-job").click();
    await page.locator("input[name=new-job-name]").fill(name);
    await page.getByTestId("create-job-button").click();
    await expect(page.locator(".notification-content")).toContainText("New job created");
}

// row is the jobs page's row of the job name
function row(page, name) {
    return page.getByTestId(name);
}

async function deleteJob(page, name) {
    const job = page.getByTestId(name);
    await job.getByTestId("delete-job-button").click();
    const refreshed = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/jobs/" && response.request().method() === "GET");
    await page.getByTestId("delete-job-confirm").click();
    const response = await refreshed;
    expect(response.status()).toBe(200);
    expect(((await response.json()) || []).map((item) => item.name)).not.toContain(name);
    await expect(job).toHaveCount(0);
}

test("should handle API network errors without crashing", async ({ page }) => {
    await page.route(
        (url) => url.pathname === "/api/jobs/",
        (route) => route.abort(),
    );
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
    await row(page, name).getByTestId("run-job-button").click();
    await page.getByTestId("start-job-confirm").click();
    const notification = page.locator(".notification-content").filter({ hasText: "has been scheduled" });
    await expect(notification).toContainText(`${name} has been scheduled`);
    await expect(notification.locator("img")).toHaveCount(0);
    expect(await page.evaluate(() => window.__xssTriggered)).toBeUndefined();
});

test("should handle reserved URL characters in a job name", async ({ page, wake }) => {
    const name = await wake.createJob(undefined, `job#?% ${Date.now()}`);
    await page.goto("/jobs");
    await row(page, name).getByTestId("run-job-button").click();
    await page.getByTestId("start-job-confirm").click();
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
    await page.getByTestId("start-job-confirm").click();
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

test("should stay in the editor when Save & Close is refused", async ({ page, wake }) => {
    const name = await wake.createJob("desc: Valid\ntasks: []\n");
    await page.goto(`/job/${name}`);
    const editor = page.locator("[data-cy=editor] .cm-content");
    await expect(editor).toContainText("desc: Valid");
    await editor.fill("tasks: [unclosed");
    const refused = page.waitForResponse((response) => response.url().endsWith(`/api/job/${name}`) && response.request().method() === "POST");
    await page.getByTestId("save-and-close-button").click();
    expect((await refused).status()).toBe(400);
    await expect(page.locator(".notification-content").last()).toBeVisible();
    await expect(page).toHaveURL(`${wake.url}/job/${name}`);
    await expect(editor).toContainText("tasks: [unclosed");
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
