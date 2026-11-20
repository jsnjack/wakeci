import { test, expect, openLog } from "./fixtures.js";

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
    await expect(page.getByTestId("task-title")).toHaveCount(1);
    await openLog(page, 0);
    await expect(page.locator("body")).toContainText("My secret is ***REDACTED***");
    await expect(page.locator("body")).toContainText("my API key is ***REDACTED***");
    await expect(page.locator("body")).toContainText("and from env ***REDACTED***");
    await expect(page.locator("body")).toContainText("Unknown secret <no value>");
    // The log viewer draws each line, its time prefix first
    const logs = page.getByTestId("task-log").locator(".cm-line");
    await expect(logs.filter({ hasText: /\] My secret is/ })).toHaveText(/^\[[^\]\n]+\] My secret is \*\*\*REDACTED\*\*\*/);
    await expect(page.getByTestId("task-log")).not.toContainText("test1");
    await expect(page.getByTestId("task-log")).not.toContainText("test2");
});
