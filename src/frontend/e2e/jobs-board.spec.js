import { randomUUID } from "node:crypto";
import { test, expect } from "./fixtures.js";

// The jobs page as a status board (J1) and the run form (J2). Each test
// filters the page to its own jobs.

test("should show each job's builds, its failure and its next run", async ({ page, wake }) => {
    const prefix = `board-${randomUUID().slice(0, 8)}`;
    const failing = await wake.createJob("tasks:\n  - name: Fail\n    run: exit 1\n", `${prefix}-failing`);
    const scheduled = await wake.createJob('interval: "0 0 1 1 *"\ntasks:\n  - name: Pass\n    run: "true"\n', `${prefix}-scheduled`);
    const first = await wake.runJob(failing);
    await wake.waitForBuild(first, "failed");
    await wake.waitForBuild(await wake.runJob(failing), "failed");
    await page.goto(`/jobs?q=${prefix}`);
    const row = page.getByTestId(failing);
    await expect(row.getByTestId("outcome-strip").locator("[data-cy-status=failed]")).toHaveCount(2);
    await expect(row.getByTestId("job-failing")).toContainText(`failing since #${first}`);
    await expect(page.getByTestId(scheduled).getByTestId("job-next-run")).toContainText("next in");
    // The filter is in the address, so it stays after a reload
    await expect(page.getByTestId("filter")).toHaveValue(prefix);
});

test("should pin jobs above the others, and sort by recent failure", async ({ page, wake }) => {
    const prefix = `pin-${randomUUID().slice(0, 8)}`;
    const a = await wake.createJob(undefined, `${prefix}-a`);
    const b = await wake.createJob("tasks:\n  - name: Fail\n    run: exit 1\n", `${prefix}-b`);
    await wake.waitForBuild(await wake.runJob(b), "failed");
    await page.goto(`/jobs?q=${prefix}`);
    const rows = page.getByTestId("jobs-container").locator(`[data-cy^="${prefix}"]`);
    await expect(rows).toHaveCount(2);
    await expect(rows.first()).toHaveAttribute("data-cy", a);
    await page.getByTestId("sort-failed").click();
    await expect(rows.first()).toHaveAttribute("data-cy", b);
    await page.getByTestId("sort-name").click();
    await page.getByTestId(b).getByTestId("pin-job").click();
    await expect(rows.first()).toHaveAttribute("data-cy", b);
    await page.reload();
    await expect(rows.first()).toHaveAttribute("data-cy", b);
});

test("should group jobs by their group or the start of their names", async ({ page, wake }) => {
    const id = randomUUID().slice(0, 8);
    await wake.createJob(undefined, `grp${id}-one`);
    await wake.createJob(undefined, `grp${id}-two`);
    await wake.createJob(`group: Deploys ${id}\ntasks:\n  - name: Hi\n    run: "true"\n`, `other${id}`);
    await page.goto(`/jobs?q=${id}`);
    await page.getByTestId("group-jobs").click();
    await expect(page.getByTestId("job-group")).toHaveText([`Deploys ${id}`, `grp${id}`]);
});

test("should explain the run form's parameters and remember the last values", async ({ page, wake }) => {
    const name = await wake.createJob(`
params:
  - TARGET: staging
  - CLEANUP: "false"
  - BRANCH_LABEL: main
param_info:
  TARGET: {desc: "Where it deploys", options: [staging, prod]}
  CLEANUP: {type: bool}
tasks:
  - name: Print
    run: echo "$TARGET $CLEANUP $BRANCH_LABEL"
`);
    await page.goto(`/jobs?q=${name}`);
    await page.getByTestId(name).getByTestId("run-job-button").click();
    const form = page.getByTestId("run-form");
    // The label first, then the job's order
    await expect(form.locator(".label-text")).toHaveText([/BRANCH_LABEL/, /TARGET/, /CLEANUP/]);
    await expect(form).toContainText("Where it deploys");
    await page.getByTestId("run-param-TARGET").fill("");
    await expect(page.getByTestId("run-empty-default")).toHaveText("Empty runs the default, staging");
    await page.getByTestId("run-param-TARGET").fill("prod");
    await page.getByTestId("run-param-CLEANUP").check();
    await expect(page.getByTestId("run-changed")).toHaveCount(2);
    await page.getByTestId("start-job-confirm").click();
    await expect(page.locator(".notification-content")).toContainText(`${name} has been scheduled`);

    // The next run starts from these values
    await page.getByTestId(name).getByTestId("run-job-button").click();
    await expect(page.getByTestId("run-last-values")).toBeVisible();
    await expect(page.getByTestId("run-param-TARGET")).toHaveValue("prod");
    await expect(page.getByTestId("run-param-CLEANUP")).toBeChecked();
    await page.getByTestId("run-use-defaults").click();
    await expect(page.getByTestId("run-param-TARGET")).toHaveValue("staging");
    await expect(page.getByTestId("run-changed")).toHaveCount(0);
});
