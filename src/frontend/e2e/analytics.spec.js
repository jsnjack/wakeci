import { randomUUID } from "node:crypto";
import { join } from "node:path";
import { test, expect } from "./fixtures.js";

// waitForEnd waits until the build's on_* and finally tasks ran, when it
// counts with its duration
async function waitForEnd(page, id) {
    await expect
        .poll(
            async () => {
                const response = await page.request.get(`/api/build/${id}`);
                await expect(response).toBeOK();
                return (await response.json()).status_update.duration > 0;
            },
            { intervals: [50, 100, 200] },
        )
        .toBe(true);
}

// pick chooses an option of a picker (SearchSelect) by typing its name and
// pressing Enter
async function pick(page, cy, text) {
    const input = page.getByTestId(cy);
    await input.click();
    await input.fill(text);
    await input.press("Enter");
}

// A job whose test task fails the first time and passes after
async function flakyJob(wake) {
    const marker = join(wake.dir, `passed-${randomUUID()}`);
    return wake.createJob(`tasks:
  - name: compile
    run: echo ok
  - name: test
    run: if [ -f '${marker}' ]; then exit 0; fi; touch '${marker}'; exit 1
`);
}

test("analytics count a job's builds and open its insights", async ({ page, wake }) => {
    const name = await flakyJob(wake);
    const failed = await wake.runJob(name);
    await wake.waitForBuild(failed, "failed");
    await waitForEnd(page, failed);
    const fixed = await wake.runJob(name);
    await wake.waitForBuild(fixed);
    await waitForEnd(page, fixed);

    // Other tests' builds share the server, so the page shows only this job
    await page.goto(`/analytics?job=${encodeURIComponent(name)}`);
    await expect(page.getByTestId("tile-builds").getByTestId("tile-value")).toHaveText("2");
    await expect(page.getByTestId("tile-success").getByTestId("tile-value")).toHaveText("50.0%");
    await expect(page.getByTestId("status-bar")).toHaveCount(30);
    const row = page.getByTestId(`job-row-${name}`);
    await expect(row.getByTestId("job-success")).toHaveText("50%");
    await expect(row.locator("[data-cy-status=failed]")).toHaveCount(1);
    await expect(row.locator("[data-cy-status=finished]")).toHaveCount(1);

    await row.getByRole("link", { name }).click();
    await expect(page).toHaveURL(`/job/${encodeURIComponent(name)}/insights`);
    await expect(page.getByTestId("task-row-test").getByTestId("task-failed")).toHaveText("1");
    await expect(page.getByTestId("task-row-compile").getByTestId("task-failed")).toHaveText("0");
    await expect(page.getByTestId("stopped-at")).toContainText("test");
    await expect(page.getByTestId("stopped-at")).toContainText("1 of 1");
    await expect(page.getByTestId("insight-flaky").getByTestId("tile-value")).toHaveText("1 of 1");
    await expect(page.getByTestId("task-bar")).toHaveCount(2);

    // A bar opens its build
    await page.getByTestId("task-bar").first().click();
    await expect(page).toHaveURL(`/build/${failed}`);
});

test("the filters stay in the address", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: broken step\n    run: exit 3\n");
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "failed");
    await waitForEnd(page, id);

    await page.goto(`/analytics?days=7&job=${encodeURIComponent(name)}`);
    await expect(page.getByTestId("range-7")).toHaveClass(/active/);
    await expect(page.getByTestId("job-filter")).toHaveValue(name);
    const row = page.getByTestId(`job-row-${name}`);
    await expect(row.getByTestId("job-success")).toHaveText("0%");
    await expect(row.locator("[data-cy-status=failed]")).toHaveCount(1);

    await page.getByTestId("range-90").click();
    await expect(page).toHaveURL(new RegExp(`days=90`));
    await expect(page.getByTestId("status-bar")).toHaveCount(90);
    await pick(page, "trigger-filter", "Schedule");
    await expect(page).toHaveURL(/trigger=interval/);
    // The build was started with the password, not the schedule
    await expect(page.getByTestId("analytics-empty")).toBeVisible();
});

test("analytics update when a build ends", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`tasks:\n  - name: wait\n    run: ${gate.command}\n`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "running");

    await wake.livePage(`/analytics?job=${encodeURIComponent(name)}`);
    await expect(page.getByTestId("analytics-empty")).toBeVisible();
    await gate.release();
    await wake.waitForBuild(id);
    // The page waits a few seconds for more builds to end before it asks
    await expect(page.getByTestId("tile-builds").getByTestId("tile-value")).toHaveText("1", { timeout: 15000 });
});

test("the jobs list and the job page lead to insights", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    await page.goto("/jobs");
    await page.getByTestId(name).getByTestId("job-insights-button").click();
    await expect(page).toHaveURL(`/job/${encodeURIComponent(name)}/insights`);
    await expect(page.getByTestId("insights-empty")).toBeVisible();
    await page.getByTestId("job-file-tab").click();
    await expect(page).toHaveURL(`/job/${encodeURIComponent(name)}`);
    await page.getByTestId("job-insights-tab").click();
    await expect(page).toHaveURL(`/job/${encodeURIComponent(name)}/insights`);
});

test("a long job name stays inside its picker on a phone", async ({ page, wake }) => {
    const job = "tasks:\n  - name: hello\n    run: echo hello\n";
    const long = await wake.createJob(job, `job-${"with-a-long-name-".repeat(4)}${randomUUID()}`);
    // The duration chart has its own picker once there are two jobs
    const other = await wake.createJob(job);
    for (const name of [long, other]) {
        const id = await wake.runJob(name);
        await wake.waitForBuild(id);
        await waitForEnd(page, id);
    }

    await page.setViewportSize({ width: 420, height: 900 });
    await page.goto("/analytics");
    const picker = page.getByTestId("duration-job");
    await pick(page, "duration-job", long);
    const card = await picker.locator("xpath=ancestor::article[1]").boundingBox();
    const field = await picker.locator("xpath=..").boundingBox();
    expect(field.x + field.width).toBeLessThanOrEqual(card.x + card.width);
    await pick(page, "job-filter", long);
    await expect(page.getByTestId("tile-builds").getByTestId("tile-value")).toHaveText("1");
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(420);
});

test("the pickers find an option as you type and fill in the rest", async ({ page, wake }) => {
    const job = "tasks:\n  - name: hello\n    run: echo hello\n";
    const id = randomUUID().slice(0, 8);
    const alpha = await wake.createJob(job, `pick-${id}-alpha-deploy`);
    const beta = await wake.createJob(job, `pick-${id}-beta-deploy`);
    await page.goto("/analytics");
    const input = page.getByTestId("job-filter");
    await input.click();
    const options = page.getByRole("listbox", { name: "Job" }).getByRole("option");
    await expect(options.first()).toHaveText("All jobs");

    // The rest of the first job that starts with the text is filled in and
    // selected, so typing goes on over it, and Enter takes it
    const typed = `pick-${id}-a`;
    await input.pressSequentially(typed);
    await expect(input).toHaveValue(alpha);
    expect(await input.evaluate((el) => [el.selectionStart, el.selectionEnd])).toEqual([typed.length, alpha.length]);
    await input.press("Enter");
    await expect(page).toHaveURL(new RegExp(`job=${alpha}`));

    // Text from the middle of a name narrows the list without filling in
    await input.click();
    await input.pressSequentially(`${id}-beta`);
    await expect(input).toHaveValue(`${id}-beta`);
    await expect(options).toHaveCount(1);
    await expect(options).toHaveText(beta);
    await input.press("Enter");
    await expect(page).toHaveURL(new RegExp(`job=${beta}`));

    // Escape puts the choice back
    await input.click();
    await input.pressSequentially("no-such-job-at-all");
    await expect(page.getByText("No match")).toBeVisible();
    await input.press("Escape");
    await expect(input).toHaveValue(beta);

    // Leaving the field takes what was filled in
    const trigger = page.getByTestId("trigger-filter");
    await trigger.click();
    await trigger.pressSequentially("sing");
    await expect(trigger).toHaveValue("Single sign-on");
    await trigger.press("Tab");
    await expect(page).toHaveURL(/trigger=sso/);
});
