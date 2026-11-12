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

test("should hide the groups from anyone but the admin password", async ({ page }) => {
    // Someone with write sees the settings, but not the groups
    await asMember(page, [{ scopes: ["read", "run", "write"], allJobs: true, jobs: [], patterns: [] }]);
    await page.goto("/settings");
    await expect(page.locator("#concurrent-builds")).toBeVisible();
    await expect(page.getByTestId("sso-groups")).toHaveCount(0);
});

test("should say that single sign-on isn't set up yet", async ({ page }) => {
    // The test servers have no identity provider
    await page.goto("/settings");
    await expect(page.getByTestId("sso-not-set-up")).toContainText("Single sign-on isn't set up on this server");
    await expect(page.getByTestId("add-group")).toBeVisible();
});

test("should show how many members of a group are signed in", async ({ page }) => {
    await page.route("**/api/groups/", (route) =>
        route.fulfill({
            json: {
                ssoEnabled: true,
                groups: [{ name: "Ops", scopes: ["read"], allJobs: true, jobs: [], patterns: [], signedIn: 4, updatedAt: new Date().toISOString() }],
            },
        }),
    );
    await page.goto("/settings");
    await expect(page.getByTestId("sso-not-set-up")).toHaveCount(0);
    await page.getByTestId("group-Ops").getByTestId("group-signed-in").click();
    await expect(page).toHaveURL(/\/keys$/);
});

test("should show an empty list of groups", async ({ page }) => {
    await page.route("**/api/groups/", (route) => route.fulfill({ json: { ssoEnabled: true, groups: [] } }));
    await page.goto("/settings");
    await expect(page.getByTestId("no-groups")).toContainText("Nobody can sign in with single sign-on until you add one");
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

test("should show a group member what a group change allows without a reload", async ({ page, wake }) => {
    const job = await wake.createJob(jobContent("deploy"), `grownjob-${randomUUID()}`);
    let socket;
    await page.routeWebSocket(/\/ws$/, (ws) => {
        ws.connectToServer();
        socket = ws;
    });
    await asMember(page, readOnly(job));
    // The page asks for the queue status once its connection is open
    const connected = page.waitForResponse((response) => response.url().endsWith("/api/queue/"));
    await page.goto("/jobs");
    await connected;
    await expect(page.getByTestId(job)).toBeVisible();
    await expect(page.getByTestId(job).getByTestId("run-job-button")).toHaveCount(0);

    // The admin lets the group run the job. The server closes the member's
    // connection, and the page reads the permissions again when it reconnects
    await asMember(page, [{ scopes: ["read", "run"], allJobs: false, jobs: [job], patterns: [] }]);
    const reloadedJobs = page.waitForResponse((response) => response.url().endsWith("/api/jobs/"));
    socket.close();
    await expect(page.getByTestId(job).getByTestId("run-job-button")).toBeVisible();
    await reloadedJobs;
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

test("should let the admin allow people in no group, and warn about write", async ({ page, backend }) => {
    await page.route("**/auth/methods", (route) => route.fulfill({ json: { sso: true } }));
    try {
        await page.goto("/settings");
        const row = page.getByTestId("no-group");
        await expect(row.getByTestId("no-group-off")).toContainText("Can't sign in");
        await row.getByTestId("allow-no-group").click();
        await expect(page).toHaveURL(/\/settings\/no-group$/);
        await expect(page.getByTestId("group-title")).toHaveText("none of the groups");

        // Read on all jobs to start with; write gets a warning
        await expect(page.getByTestId("save-group")).toHaveText("Allow");
        await page.getByTestId("scope-write").check({ force: true });
        await expect(page.getByTestId("no-group-write-warning")).toBeVisible();
        await page.getByTestId("scope-write").uncheck({ force: true });
        await expect(page.getByTestId("no-group-write-warning")).toHaveCount(0);
        await page.getByTestId("save-group").click();
        await expect(page).toHaveURL(/\/settings$/);
        await expect(row.getByTestId("no-group-scope-read")).toBeVisible();
        await expect(row.getByTestId("no-group-jobs")).toHaveText("All jobs");
        await expect(row.getByTestId("no-group-signed-in")).toHaveText("nobody signed in");

        // Editing starts from the rule
        await row.getByTestId("edit-no-group").click();
        await expect(page.getByTestId("save-group")).toHaveText("Save");
        await page.getByTestId("scope-run").check({ force: true });
        await page.getByTestId("save-group").click();
        await expect(row.getByTestId("no-group-scope-run")).toBeVisible();

        await row.getByTestId("turn-off-no-group").click();
        await page.getByTestId("turn-off-no-group-confirm").click();
        await expect(row.getByTestId("no-group-off")).toBeVisible();
        expect((await (await backend.api.get("/api/groups/")).json()).noGroup).toBeNull();
    } finally {
        // Other tests of this worker use the same wakeci
        await backend.api.delete("/api/nogroup");
    }
});
