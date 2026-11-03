import { test, expect } from "./fixtures.js";

test("should use secrets and not expose them", async ({ page, wake }) => {
    const jobContent = `
desc: New job
tasks:
  - name: Example of using secrets
    run: echo "My secret is {{ secrets.TEST_KEY_1 }} and my API key is {{secrets.TEST_KEY_2 }} and from env $API_KEY. Unknown secret {{secrets.UNKNOWN_KEY }}"
    env:
      API_KEY: "{{     secrets.TEST_KEY_1 }}"
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(1);
    await page.locator("[data-cy=reload]").nth(0).click();
    await expect(page.locator("body")).toContainText("My secret is ***REDACTED***");
    await expect(page.locator("body")).toContainText("my API key is ***REDACTED***");
    await expect(page.locator("body")).toContainText("and from env ***REDACTED***");
    await expect(page.locator("body")).toContainText("Unknown secret <no value>");
});
