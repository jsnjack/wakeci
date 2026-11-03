import { test, expect } from "./fixtures.js";

test("should not ignore errors if task fails", async ({ page, wake }) => {
    const jobContent = `
desc: Ignore errors test
tasks:
  - name: Print env variable
    run: printenv NON_EXISTING_VARIABLE
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id, "failed");
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("> Exit code: 1");
    await expect(page.locator("body")).toContainText("failed");
});

test("should ignore errors if task fails", async ({ page, wake }) => {
    const jobContent = `
desc: Ignore errors test
tasks:
  - name: Print env variable
    run: printenv NON_EXISTING_VARIABLE
    ignore_errors: yes
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("> Exit code: 1");
    await expect(page.locator("body")).toContainText("> Ignorring exit code");
    await expect(page.locator("body")).not.toContainText("failed");
});
