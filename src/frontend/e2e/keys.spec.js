import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { test, expect } from "./fixtures.js";

const jobContent = (name) => `desc: ${name}\ntasks:\n  - name: Say hi\n    run: echo hi from ${name}\n`;

// keyAPI sends requests with nothing but the API key: no session cookie
async function keyAPI(playwright, backend, token) {
    return playwright.request.newContext({
        baseURL: backend.url,
        extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
}

const fakeKey = (id, name, createdBy, scopes = ["read"]) => ({
    id,
    name,
    createdBy,
    scopes,
    allJobs: true,
    jobs: [],
    patterns: [],
    createdAt: new Date().toISOString(),
});

test("should create a key that only works on its jobs", async ({ page, wake, backend, playwright }) => {
    const deploy = await wake.createJob(jobContent("deploy"), `keydeploy-${randomUUID()}`);
    const other = await wake.createJob(jobContent("other"), `keyother-${randomUUID()}`);
    const name = `deploy ${randomUUID()}`;

    await page.goto("/keys");
    await expect(page.getByTestId("nav-keys")).toBeVisible();
    await page.getByTestId("create-key-open").click();
    await expect(page).toHaveURL(/\/keys\/new$/);
    await page.getByTestId("key-name").fill(name);
    await page.getByTestId("scope-run").check({ force: true });
    await page.getByTestId("jobs-selected").check({ force: true });
    await page.getByTestId(`key-job-${deploy}`).locator("input").check({ force: true });
    await expect(page.getByTestId("key-jobs-count")).toContainText("1 of");
    await page.getByTestId("create-key").click();

    const tokenInput = page.getByTestId("new-key-token");
    await expect(tokenInput).toHaveValue(/^wk_[0-9a-f]{12}_[A-Za-z0-9_-]{43}$/);
    const token = await tokenInput.inputValue();
    await expect(page.getByTestId("new-key-example")).toContainText("/api/feed");

    const api = await keyAPI(playwright, backend, token);
    try {
        const jobs = await api.get("/api/jobs/");
        expect(jobs.status()).toBe(200);
        expect((await jobs.json()).map((job) => job.name)).toEqual([deploy]);
        expect((await api.post(`/api/job/${other}/run`)).status()).toBe(403);
        expect((await api.get("/api/settings")).status()).toBe(403);
        expect((await api.get("/api/keys/")).status()).toBe(403);
        expect((await api.post(`/api/job/${deploy}/run`, { form: { X: "{{ secrets.A }}" } })).status()).toBe(400);

        const run = await api.post(`/api/job/${deploy}/run`);
        expect(run.status()).toBe(200);
        expect((await api.get(`/api/build/${await run.json()}`)).status()).toBe(200);
        const otherBuild = await wake.runJob(other);
        expect((await api.get(`/api/build/${otherBuild}`)).status()).toBe(403);
        expect((await api.get(`/storage/build/${otherBuild}/`)).status()).toBe(403);
    } finally {
        await api.dispose();
    }

    await page.getByTestId("new-key-done").click();
    await expect(page).toHaveURL(/\/keys$/);
    const card = page.getByTestId(`key-${name}`);
    await expect(card).toContainText("read");
    await expect(card).toContainText("run");
    await expect(card.getByTestId("key-jobs")).toHaveText(deploy);
    await expect(card).toContainText("Used");
});

test("should select jobs by pattern", async ({ page, wake }) => {
    const prefix = `pattern${randomUUID().slice(0, 8)}`;
    const matching = await wake.createJob(jobContent("matching"), `${prefix}-deploy`);
    const other = await wake.createJob(jobContent("other"), `other-${randomUUID()}`);

    await page.goto("/keys/new");
    await page.getByTestId("jobs-selected").check({ force: true });
    await page.getByTestId("key-patterns").fill(`${prefix}*`);
    await expect(page.getByTestId(`key-job-${matching}`)).toContainText(`${prefix}*`);
    await expect(page.getByTestId(`key-job-${matching}`).locator("input")).toBeChecked();
    await expect(page.getByTestId(`key-job-${matching}`).locator("input")).toBeDisabled();
    await expect(page.getByTestId(`key-job-${other}`).locator("input")).not.toBeChecked();
    await expect(page.getByTestId(`key-job-${other}`).locator("input")).toBeEnabled();
});

test("should keep write keys on all jobs", async ({ page }) => {
    await page.goto("/keys/new");
    await page.getByTestId("jobs-selected").check({ force: true });
    await expect(page.getByTestId("key-patterns")).toBeVisible();
    await page.getByTestId("scope-write").check({ force: true });
    await expect(page.getByTestId("jobs-all")).toBeChecked();
    await expect(page.getByTestId("jobs-selected")).toBeDisabled();
    await expect(page.getByTestId("key-patterns")).toHaveCount(0);

    // Unticking write gives the job choice back
    await page.getByTestId("scope-write").uncheck({ force: true });
    await expect(page.getByTestId("jobs-selected")).toBeChecked();
    await expect(page.getByTestId("jobs-selected")).toBeEnabled();
    await expect(page.getByTestId("key-patterns")).toBeVisible();
});

test("should only check the job fields while they are shown", async ({ page }) => {
    await page.goto("/keys/new");
    await page.getByTestId("key-name").fill(`hidden ${randomUUID()}`);
    await page.getByTestId("jobs-selected").check({ force: true });
    await page.getByTestId("key-patterns").fill("bad/pattern");
    await page.getByTestId("create-key").click();
    await expect(page.locator("body")).toContainText("Patterns can't contain");
    await expect(page.getByTestId("new-key-token")).toHaveCount(0);

    await page.getByTestId("jobs-all").check({ force: true });
    await page.getByTestId("create-key").click();
    await expect(page.getByTestId("new-key-token")).toBeVisible();
});

test("should revoke a key", async ({ page, backend, playwright }) => {
    const name = `revoke ${randomUUID()}`;
    const created = await backend.api.post("/api/keys/", {
        form: { name, scope: "read", allJobs: "true", expiresInDays: "0" },
    });
    await expect(created).toBeOK();
    const api = await keyAPI(playwright, backend, (await created.json()).token);
    try {
        expect((await api.get("/api/jobs/")).status()).toBe(200);

        await page.goto("/keys");
        // Made while logged in with the admin password, so listed under it
        const card = page.getByTestId("key-group-admin-password").getByTestId(`key-${name}`);
        await expect(card).toContainText("Never expires");
        await card.getByTestId("revoke-key").click();
        await page.getByTestId("revoke-key-confirm").click();
        await expect(page.locator(".notification-content")).toContainText("Key revoked");
        await expect(card).toHaveCount(0);
        expect((await api.get("/api/jobs/")).status()).toBe(401);
    } finally {
        await api.dispose();
    }
});

test("should let the admin password revoke the access of a person", async ({ page }) => {
    const revoked = [];
    await page.route(
        (url) => url.pathname === "/api/keys/",
        (route) => {
            if (route.request().method() === "DELETE") {
                const person = new URL(route.request().url()).searchParams.get("createdBy");
                revoked.push(person);
                return route.fulfill({ json: { keys: person === "alice@example.com" ? 2 : 0, sessions: 1 } });
            }
            return route.fulfill({
                json: {
                    admin: true,
                    // carol is signed in but has no keys
                    people: ["alice@example.com", "bob@example.com", "carol@example.com"],
                    signedIn: ["alice@example.com", "carol@example.com"],
                    keys: [
                        fakeKey("000000000001", "alice-ci", "alice@example.com"),
                        fakeKey("000000000002", "alice-bot", "alice@example.com"),
                        fakeKey("000000000003", "bob-ci", "bob@example.com"),
                        fakeKey("000000000004", "shared", ""),
                    ],
                },
            });
        },
    );
    await page.goto("/keys");

    // Keys made with the admin password come first and are revoked one by one
    const password = page.getByTestId("key-group-admin-password");
    await expect(password).toContainText("Made with the admin password");
    await expect(password.getByTestId("key-shared")).toBeVisible();
    await expect(password.getByTestId("revoke-person")).toHaveCount(0);

    const alice = page.getByTestId("key-group-alice@example.com");
    await expect(alice.getByTestId("key-alice-ci")).toBeVisible();
    await expect(alice.getByTestId("key-alice-bot")).toBeVisible();
    await expect(alice).not.toContainText("bob-ci");
    await expect(alice.getByTestId("key-group-status")).toHaveText("· signed in");
    await expect(page.getByTestId("key-group-bob@example.com").getByTestId("key-group-status")).toHaveCount(0);
    await alice.getByTestId("revoke-person").click();
    await expect(page.locator("dialog.active")).toContainText("revokes their 2 keys");
    await page.getByTestId("revoke-person-confirm").click();
    await expect(page.locator(".notification-content").filter({ hasText: "Signed out alice@example.com and revoked 2 keys" })).toBeVisible();

    // Someone signed in without keys can still be signed out
    const carol = page.getByTestId("key-group-carol@example.com");
    await expect(carol.getByTestId("key-group-status")).toHaveText("· signed in, no keys");
    await carol.getByTestId("revoke-person").click();
    await page.getByTestId("revoke-person-confirm").click();
    await expect(page.locator(".notification-content").filter({ hasText: "Signed out carol@example.com and revoked 0 keys" })).toBeVisible();
    expect(revoked).toEqual(["alice@example.com", "carol@example.com"]);
});

test("should search the keys of many people", async ({ page }) => {
    const people = ["ann@example.com", "ben@example.com", "cat@example.com", "dan@example.com", "zed@example.com"];
    await page.route(
        (url) => url.pathname === "/api/keys/",
        (route) =>
            route.fulfill({
                json: { admin: true, people, signedIn: [], keys: people.map((person, i) => fakeKey(`00000000000${i}`, `${person.split("@")[0]}-ci`, person)) },
            }),
    );
    await page.goto("/keys");
    await page.getByTestId("key-search").fill("zed");
    await expect(page.getByTestId("key-group-zed@example.com")).toBeVisible();
    await expect(page.getByTestId("key-group-ann@example.com")).toHaveCount(0);
    // Key names match too
    await page.getByTestId("key-search").fill("ben-ci");
    await expect(page.getByTestId("key-group-ben@example.com")).toBeVisible();
    await page.getByTestId("key-search").fill("nobody");
    await expect(page.getByTestId("keys-list")).toContainText('Nothing matches "nobody"');
});

test("should show an SSO user only their own keys", async ({ page }) => {
    await page.route(
        (url) => url.pathname === "/api/keys/",
        (route) => route.fulfill({ json: { admin: false, keys: [fakeKey("000000000001", "my-ci", "alice@example.com", ["run"])] } }),
    );
    await page.goto("/keys");
    await expect(page.getByTestId("keys-list")).toContainText("my-ci");
    // No headings, no other people, nothing to revoke but their own keys
    await expect(page.getByTestId("revoke-person")).toHaveCount(0);
    await expect(page.getByTestId("key-group-status")).toHaveCount(0);
    await expect(page.getByTestId("keys-list")).not.toContainText("alice@example.com");
});

test("should give a run-only key a working curl example", async ({ page, wake }) => {
    // Valid job names can hold characters the shell treats specially
    const job = await wake.createJob(jobContent("legacy"), `release(legacy)'s-${randomUUID()}`);
    await page.goto("/keys/new");
    await page.getByTestId("key-name").fill(`run ${randomUUID()}`);
    await page.getByTestId("scope-read").uncheck({ force: true });
    await page.getByTestId("scope-run").check({ force: true });
    await page.getByTestId("jobs-selected").check({ force: true });
    await page.getByTestId(`key-job-${job}`).locator("input").check({ force: true });
    await page.getByTestId("create-key").click();

    const example = await page.getByTestId("new-key-example").innerText();
    expect(example).toContain(`/api/job/${encodeURIComponent(job).replaceAll("'", "%27")}/run'`);
    // bash -n only parses: it throws on a syntax error and runs nothing
    execFileSync("bash", ["-n", "-c", example]);
});

test("should show how much each key is used, per day", async ({ page, backend, playwright }) => {
    const name = `stats-${randomUUID()}`;
    const created = await backend.api.post("/api/keys/", { form: { name, scope: "read", allJobs: "true", expiresInDays: "1" } });
    await expect(created).toBeOK();
    const key = await playwright.request.newContext({
        baseURL: backend.url,
        storageState: { cookies: [], origins: [] },
        extraHTTPHeaders: { Authorization: `Bearer ${(await created.json()).token}` },
    });
    await expect(await key.get("/api/whoami")).toBeOK();
    await expect(await key.get("/api/jobs/")).toBeOK();
    await key.dispose();

    await page.goto("/keys");
    const row = page.getByTestId(`key-${name}`);
    await expect(row.getByTestId("key-uses")).toHaveText("2 requests");
    await expect(row.getByTestId("key-last-used")).toContainText("Used");
    await expect(row.getByTestId("key-usage-bar")).toHaveCount(30);
    // Today is the last bar
    await expect(row.getByTestId("key-usage-bar").last().locator("title")).toHaveText(/: 2 requests$/);
    await expect(row.getByTestId("key-usage-total")).toHaveText("2 in 30 days");
    await expect(row.getByTestId("key-usage").locator("svg")).toHaveAttribute("aria-label", /^2 requests in the last 30 days/);
});
