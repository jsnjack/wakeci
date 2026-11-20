import { test, expect } from "./fixtures.js";

// The feed's quick filters and saved searches (F4), and help while typing
// (F8). Each test starts from a search for its own job, so the other tests'
// builds stay out.

test("should filter by status with the quick filters", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
params:
  - FAIL: "no"
tasks:
  - name: Maybe wait
    run: test "$FAIL" = wait && ${gate.command} || test "$FAIL" = "no"
`);
    const passed = await wake.runJob(name);
    const failed = await wake.runJob(name, { FAIL: "yes" });
    await wake.waitForBuild(passed);
    await wake.waitForBuild(failed, "failed");
    const running = await wake.runJob(name, { FAIL: "wait" });
    await wake.waitForBuild(running, "running");
    await wake.showFeed(name);
    const builds = page.locator("[data-cy-build]");
    await expect(builds).toHaveCount(3);

    await page.getByTestId("filter-failed").click();
    await expect(page.getByTestId("filter")).toHaveValue(`${name} +status=failed`);
    await expect(builds).toHaveCount(1);
    await expect(page.locator(`[data-cy-build="${failed}"]`)).toBeVisible();
    await page.getByTestId("filter-failed").click();
    await expect(page.getByTestId("filter")).toHaveValue(name);

    await page.getByTestId("filter-active").click();
    await expect(page.getByTestId("filter-active")).toHaveAttribute("aria-pressed", "true");
    await expect(builds).toHaveCount(1);
    await expect(page.locator(`[data-cy-build="${running}"]`)).toBeVisible();
    await gate.release();
});

test("should hide scheduled jobs and pick jobs", async ({ page, wake }) => {
    const scheduled = await wake.createJob(`
interval: "0 0 1 1 *"
tasks:
  - name: Rare
    run: "true"
`);
    const other = await wake.createJob();
    await page.goto("/");
    await page.getByTestId("filter-scheduled").click();
    await expect(page.getByTestId("filter")).toHaveValue(new RegExp(`-name=${scheduled}`));
    await page.getByTestId("filter").fill("");
    await page.getByTestId("filter-jobs").click();
    await page.getByRole("menuitemcheckbox", { name: other }).click();
    await expect(page.getByTestId("filter")).toHaveValue(`name=${other}`);
    await expect(page.getByTestId("filter-jobs")).toContainText("1 job");
});

test("should save a search, keep it, and forget it", async ({ page, wake }) => {
    const name = await wake.createJob();
    await page.goto("/");
    await page.getByTestId("filter").fill(`+name=${name}`);
    await page.getByTestId("save-search").click();
    await page.getByTestId("save-search-name").fill("Mine");
    await page.getByTestId("save-search-confirm").click();
    const saved = page.getByTestId("saved-search").filter({ hasText: "Mine" });
    await expect(saved).toHaveAttribute("aria-pressed", "true");
    await page.getByTestId("filter").fill("");
    await page.reload();
    await saved.click();
    await expect(page.getByTestId("filter")).toHaveValue(`+name=${name}`);
    await page.getByTestId("forget-search").click();
    await expect(page.getByTestId("saved-search")).toHaveCount(0);
});

test("should suggest names and values while typing, and remove terms", async ({ page, wake }) => {
    const name = await wake.createJob(`
params:
  - BRANCH_LABEL: main
tasks:
  - name: Print
    run: "true"
`);
    await wake.runJob(name, { BRANCH_LABEL: "T-77-suggest" });
    await wake.showFeed(name);
    await expect(page.locator("[data-cy-build]")).toHaveCount(1);
    const input = page.getByTestId("filter");
    await input.press("End");
    await input.pressSequentially(" bra");
    await expect(page.getByTestId("search-suggestions")).toContainText("branch=");
    await input.press("ArrowDown");
    await input.press("Enter");
    await expect(input).toHaveValue(`${name} branch=`);
    await expect(page.getByTestId("search-suggestions")).toContainText("branch=T-77-suggest");
    await input.press("ArrowDown");
    await input.press("Enter");
    await expect(input).toHaveValue(`${name} branch=T-77-suggest `);
    // The terms show one by one, to remove
    await expect(page.getByTestId("search-terms")).toContainText("branch=T-77-suggest");
    await page.getByTestId("remove-term").first().click();
    await expect(input).toHaveValue("branch=T-77-suggest");
});
