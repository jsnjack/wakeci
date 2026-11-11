import { randomUUID } from "node:crypto";
import { test, expect } from "./fixtures.js";

const jobContent = (name) => `desc: ${name}\ntasks:\n  - name: Say hi\n    run: echo hi from ${name}\n`;

// asMember makes the page believe it is logged in as a single sign-on user
// whose groups allow grants. The backend still sees the admin session: these
// tests check what the pages show, the Go tests what the API allows.
async function asMember(page, grants) {
    await page.route("**/auth/_isLoggedIn", (route) => route.fulfill({ json: { user: "sam@example.com", admin: false, grants } }));
}

const readOnly = (job) => [{ scopes: ["read"], allJobs: false, jobs: [job], patterns: [] }];

test("should let the admin add, change and remove a single sign-on group", async ({ page, wake }) => {
    await page.route("**/auth/methods", (route) => route.fulfill({ json: { sso: true } }));
    const job = await wake.createJob(jobContent("deploy"), `groupjob-${randomUUID()}`);
    const group = `Service Team ${randomUUID()}`;

    await page.goto("/settings");
    await page.getByTestId("add-group").click();
    await expect(page).toHaveURL(/\/settings\/groups\/new$/);

    // Every missing field is pointed out at once
    await page.getByTestId("save-group").click();
    await expect(page.getByText("Enter the group's name")).toBeVisible();
    await expect(page.getByText("Select a job or enter a pattern")).toBeVisible();

    await page.getByTestId("group-name").fill(group);
    await page.getByTestId(`group-job-${job}`).locator("input").check({ force: true });
    await page.getByTestId("save-group").click();
    await expect(page).toHaveURL(/\/settings$/);
    const row = page.getByTestId(`group-${group}`);
    await expect(row.getByTestId("group-jobs")).toHaveText(job);
    await expect(row.getByTestId("group-scope-read")).toBeVisible();
    await expect(row.getByTestId("group-scope-run")).toHaveCount(0);

    // Editing starts from what the group may do now
    await row.getByTestId("edit-group").click();
    await expect(page.getByTestId("group-title")).toHaveText(group);
    await expect(page.getByTestId(`group-job-${job}`).locator("input")).toBeChecked();
    await page.getByTestId("scope-run").check({ force: true });
    await page.getByTestId("group-patterns").fill("nightly_*");
    await page.getByTestId("save-group").click();
    await expect(row.getByTestId("group-scope-run")).toBeVisible();
    await expect(row.getByTestId("group-jobs")).toHaveText(`${job}, nightly_*`);

    await row.getByTestId("remove-group").click();
    await page.getByTestId("remove-group-confirm").click();
    await expect(row).toHaveCount(0);
});

test("should refuse a group name that exists in another case", async ({ page, backend }) => {
    await page.route("**/auth/methods", (route) => route.fulfill({ json: { sso: true } }));
    const group = `Ops ${randomUUID()}`;
    await expect(await backend.api.post("/api/groups/", { form: { name: group, scope: "read", allJobs: "true" } })).toBeOK();

    await page.goto("/settings/groups/new");
    await page.getByTestId("group-name").fill(group.toUpperCase());
    await page.getByTestId("jobs-all").check({ force: true });
    await page.getByTestId("save-group").click();
    await expect(page.getByText("exists already")).toBeVisible();
    await expect(page).toHaveURL(/\/settings\/groups\/new$/);

    await expect(await backend.api.delete(`/api/groups/${encodeURIComponent(group)}`)).toBeOK();
});

test("should show the groups read-only to anyone but the admin password", async ({ page }) => {
    await page.route("**/auth/methods", (route) => route.fulfill({ json: { sso: true } }));
    await page.route("**/api/groups/", (route) =>
        route.fulfill({
            json: {
                canEdit: false,
                groups: [{ name: "Ops", scopes: ["read", "run", "write"], allJobs: true, jobs: [], patterns: [], updatedAt: new Date().toISOString() }],
            },
        }),
    );
    await page.goto("/settings");
    await expect(page.getByTestId("group-Ops").getByTestId("group-jobs")).toHaveText("All jobs");
    await expect(page.getByTestId("add-group")).toHaveCount(0);
    await expect(page.getByTestId("edit-group")).toHaveCount(0);
    await expect(page.getByTestId("remove-group")).toHaveCount(0);
    await expect(page.getByText("Only the admin password can change the groups")).toBeVisible();
});

test("should only offer a group member what their groups allow", async ({ page, wake }) => {
    const job = await wake.createJob(jobContent("deploy"), `memberjob-${randomUUID()}`);
    const build = await wake.runJob(job);
    await asMember(page, readOnly(job));

    await page.goto("/jobs");
    const row = page.getByTestId(job);
    await expect(row).toBeVisible();
    await expect(row.getByTestId("run-job-button")).toHaveCount(0);
    await expect(row.getByTestId("delete-job-button")).toHaveCount(0);
    await expect(row.getByTestId("edit-job-button")).toContainText("View");
    await expect(page.getByTestId("create-job")).toHaveCount(0);
    await expect(page.getByTestId("nav-settings")).toHaveCount(0);
    await expect(page.getByTestId("nav-keys")).toBeVisible();

    // The job's definition can be read, not changed
    await row.getByTestId("edit-job-button").click();
    await expect(page.getByTestId("editor")).toContainText("hi from deploy");
    await expect(page.getByTestId("save-button")).toHaveCount(0);
    await expect(page.getByTestId("editor").locator(".cm-content")).toHaveAttribute("contenteditable", "false");

    await page.goto(`/build/${build}`);
    await expect(page.getByTestId("abort-build-button")).toHaveCount(0);
    await expect(page.getByTestId("run-job-button")).toHaveCount(0);
});

test("should keep the run buttons for a group member who may run the job", async ({ page, wake }) => {
    const job = await wake.createJob(jobContent("deploy"), `runnerjob-${randomUUID()}`);
    await asMember(page, [{ scopes: ["read", "run"], allJobs: false, jobs: [], patterns: ["runnerjob-*"] }]);

    await page.goto("/jobs");
    await expect(page.getByTestId(job).getByTestId("run-job-button")).toBeVisible();
    await expect(page.getByTestId(job).getByTestId("delete-job-button")).toHaveCount(0);
});

test("should only offer a group member keys within their groups", async ({ page, wake }) => {
    const job = await wake.createJob(jobContent("deploy"), `memberkey-${randomUUID()}`);
    await asMember(page, readOnly(job));

    await page.goto("/keys/new");
    await expect(page.getByTestId("scope-read")).toBeChecked();
    await expect(page.getByTestId("scope-run")).toHaveCount(0);
    await expect(page.getByTestId("scope-write")).toHaveCount(0);
    await expect(page.getByText("All jobs your groups allow, including jobs added later")).toBeVisible();
});

test("should stay logged in when the groups don't allow a request", async ({ page, wake }) => {
    const job = await wake.createJob(jobContent("deploy"), `refusedjob-${randomUUID()}`);
    await asMember(page, [{ scopes: ["read", "run"], allJobs: false, jobs: [job], patterns: [] }]);
    await page.route(
        (url) => url.pathname === `/api/job/${job}/run`,
        (route) => route.fulfill({ status: 403, contentType: "text/plain", body: "Your account doesn't have the run scope" }),
    );

    await page.goto("/jobs");
    await page.getByTestId(job).getByTestId("run-job-button").click();
    await page.getByTestId("start-job-confirm").click();
    await expect(page.getByText("Your account doesn't have the run scope")).toBeVisible();
    await expect(page).toHaveURL(/\/jobs$/);
});

test("should go to the login page when the session ended", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("header h6")).toHaveText("Feed");
    // The session ends: every request, the login check too, gets 401
    const ended = (route) =>
        route.fulfill({
            status: 401,
            contentType: "text/plain",
            headers: { "WWW-Authenticate": 'Bearer realm="wakeci"' },
            body: "The session ended; log in again",
        });
    await page.route("**/api/jobs/", ended);
    await page.route("**/auth/_isLoggedIn", ended);
    await page.locator('header a[href="/jobs"]').click();
    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByText("The session ended; log in again")).toBeVisible();
});
