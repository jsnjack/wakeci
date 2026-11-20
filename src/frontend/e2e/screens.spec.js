import { join } from "node:path";
import { readFile, writeFile } from "node:fs/promises";
import { test as base, expect, launchBackend } from "./fixtures.js";

// Screenshots of every page at three widths, light and dark, compared with
// how the pages look on the base commit:
//
//   make screens                  # against master
//   make screens BASE=<commit>    # against another commit
//
// It builds the base in a temporary worktree and takes the baselines there,
// with this spec, so the base needs the test IDs the spec uses. Anything a
// change moves shows in `make testreport`.
//
// Times, IDs, the version and log text are masked, since they differ on
// every run; the rest comes from a backend of the spec's own with the same
// jobs, builds, group and key each time. Analytics and job insights show
// recorded answers (screens-*.json) at the time they were recorded.

const sizes = [
    { name: "1440", width: 1440, height: 900 },
    { name: "1024", width: 1024, height: 768 },
    { name: "390", width: 390, height: 844 },
];
const themes = ["light", "dark"];

const jobs = {
    deploy: `desc: Deploy a branch to an environment
params:
  - BRANCH_LABEL: main
  - TARGET: staging
tasks:
  - name: Build
    run: echo "Building $BRANCH_LABEL"; echo "Built image app:$BRANCH_LABEL"
  - name: Deploy
    run: echo "Deploying to $TARGET"; test "$BRANCH_LABEL" = main
`,
    "nightly-backup": `desc: Back up the database
interval: "0 3 * * *"
tasks:
  - name: Back up
    run: echo "Backed up 12 tables"
`,
    tests: (gate) => `desc: Run the test suite
tasks:
  - name: Unit tests
    run: echo "ok  pkg/api"; echo "ok  pkg/db"
  - name: End-to-end tests
    run: echo "Running 24 tests"; ${gate}
`,
};

async function createJob(api, name, content) {
    await expect(await api.post(`/api/job/${name}`, { form: { fileContent: content } })).toBeOK();
    await expect
        .poll(async () => ((await (await api.get("/api/jobs/")).json()) || []).some((job) => job.name === name), { intervals: [10, 25, 50] })
        .toBe(true);
}

async function runJob(api, name, status, params) {
    const response = await api.post(`/api/job/${name}/run`, params ? { form: params } : undefined);
    await expect(response).toBeOK();
    const id = await response.json();
    await expect.poll(async () => (await (await api.get(`/api/build/${id}`)).json()).status_update.status, { intervals: [50, 100, 200] }).toBe(status);
    return id;
}

// seed adds what the pages show. Builds run one at a time, so a new
// database gives them the same IDs: build 2 failed and build 4 runs until
// the backend stops
async function seed(backend) {
    const { api, root } = backend;
    await expect(await api.post("/api/groups/", { form: { name: "developers", scope: "run", allJobs: "true" } })).toBeOK();
    const key = await api.post("/api/keys/", { form: { name: "ci-bot", scope: "run", allJobs: "true", expiresInDays: "0" } });
    await expect(key).toBeOK();
    const keyCreatedAt = new Date((await key.json()).key.createdAt);
    const gate = join(root, "release");
    await createJob(api, "deploy", jobs.deploy);
    await createJob(api, "nightly-backup", jobs["nightly-backup"]);
    await createJob(api, "tests", jobs.tests(`while [ ! -f '${gate}' ] && [ -d '${root}' ]; do sleep 0.05; done`));
    await runJob(api, "deploy", "finished");
    await runJob(api, "deploy", "failed", { BRANCH_LABEL: "T-4821-fix-login-redirect" });
    await runJob(api, "nightly-backup", "finished");
    await runJob(api, "tests", "running");
    return { release: () => writeFile(gate, ""), keyCreatedAt };
}

const test = base.extend({
    shots: [
        async ({ playwright }, use) => {
            const backend = await launchBackend(playwright);
            let seeded;
            try {
                seeded = await seed(backend);
                await use({ ...backend, ...seeded });
            } finally {
                // A stop waits for running builds
                if (seeded) await seeded.release();
                await backend.stop();
            }
        },
        { scope: "worker" },
    ],
    baseURL: async ({ shots }, use) => use(shots.url),
    storageState: async ({ shots }, use) => use(shots.storageState),
});
test.use({ locale: "en-US", timezoneId: "UTC" });

// answerFrom answers requests for pathname with the answer recorded in
// file, and sets the page's clock to when it was recorded, so its days and
// "just now" stay as they were
function answerFrom(file, pathname) {
    return async (page) => {
        const { now, data } = JSON.parse(await readFile(join(import.meta.dirname, file), "utf8"));
        await page.clock.setFixedTime(new Date(now));
        await page.route(
            (url) => url.pathname === pathname,
            (route) => route.fulfill({ json: data }),
        );
    };
}

// Each page: where it is, and what shows once it has loaded
const pages = [
    { name: "login", path: "/login", signedOut: true, ready: (page) => page.locator("#password") },
    { name: "feed", path: "/", ready: (page) => page.locator("[data-cy-build]").nth(3) },
    {
        name: "build-failed",
        path: "/build/2",
        // Logs open on a click; the failed task's shows its text masked
        open: (page) => page.getByTestId("task-title").nth(1).click(),
        ready: (page) => page.getByTestId("task-log"),
    },
    // The running task shows only lines printed after the page opened (L2)
    { name: "build-running", path: "/build/4", ready: (page) => page.getByTestId("task-title").nth(1) },
    { name: "jobs", path: "/jobs", ready: (page) => page.getByTestId("tests") },
    {
        name: "run-form",
        path: "/jobs",
        ready: (page) => page.getByTestId("deploy").getByTestId("start-job-confirm"),
        open: (page) => page.getByTestId("deploy").getByTestId("run-job-button").click(),
        // The dialog covers the window; the page under it scrolls
        fullPage: false,
    },
    { name: "job-editor", path: "/job/deploy", ready: (page) => page.getByTestId("editor") },
    {
        name: "job-insights",
        path: "/job/deploy/insights",
        prepare: answerFrom("screens-insights.json", "/api/job/deploy/insights"),
        ready: (page) => page.getByTestId("job-insights-tab"),
    },
    {
        name: "analytics",
        path: "/analytics",
        prepare: answerFrom("screens-analytics.json", "/api/analytics/"),
        ready: (page) => page.getByTestId("tile-builds").getByTestId("tile-value"),
    },
    { name: "settings", path: "/settings", ready: (page) => page.getByTestId("version") },
    { name: "group", path: "/settings/groups/developers", ready: (page) => page.getByTestId("save-group") },
    { name: "group-new", path: "/settings/groups/new", ready: (page) => page.getByTestId("save-group") },
    { name: "no-group", path: "/settings/no-group", ready: (page) => page.getByTestId("save-group") },
    {
        name: "keys",
        path: "/keys",
        // "Created just now" stays so however long the run takes
        prepare: (page, shots) => page.clock.setFixedTime(shots.keyCreatedAt),
        ready: (page) => page.getByTestId("key-ci-bot"),
    },
    { name: "key-new", path: "/keys/new", ready: (page) => page.getByTestId("create-key") },
    // The job syntax, the first tab
    { name: "help", path: "/help", ready: (page) => page.getByText("Ask a cow to say something smart") },
];

const masked = ["duration", "started-ago", "app-version", "spinner", "task-log", "key-id", "key-last-used"];

for (const theme of themes) {
    for (const shot of pages) {
        test(`${shot.name} looks the same, ${theme}`, async ({ page, shots }) => {
            await page.addInitScript((theme) => {
                window.localStorage.setItem("theme", theme);
                // Following the log scrolls the page, at its own time
                window.localStorage.setItem("follow", "false");
            }, theme);
            if (shot.signedOut) await page.context().clearCookies();
            if (shot.prepare) await shot.prepare(page, shots);
            for (const size of sizes) {
                await page.setViewportSize({ width: size.width, height: size.height });
                await page.goto(shot.path);
                if (shot.open) await shot.open(page);
                await expect(shot.ready(page)).toBeVisible();
                await page.evaluate(() => document.fonts.ready);
                await expect(page).toHaveScreenshot(`${shot.name}-${size.name}-${theme}.png`, {
                    fullPage: shot.fullPage ?? true,
                    mask: masked.map((id) => page.getByTestId(id)),
                });
            }
        });
    }
}
