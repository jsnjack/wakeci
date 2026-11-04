import { test, expect } from "./fixtures.js";

test("should abort build when a user asks from the feed page", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
desc: Abort test
tasks:
  - name: Wait for test
    run: ${gate.command}
`);
    const id = await wake.runJob(name);
    await wake.showFeed(name);
    await expect(page.locator("[data-cy-status]")).toHaveAttribute("data-cy-status", "running");
    await page.getByTestId("abort-build-button").click();
    await expect(page.locator("[data-cy-status]")).toHaveAttribute("data-cy-status", "aborted");
    await page.getByTestId("open-build-button").click();
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy-status]")).toHaveCount(2);
    await expect(page.locator("[data-cy-status]").nth(1)).toHaveAttribute("data-cy-status", "aborted");
    await page.getByTestId("reload").click();
    await expect(page.locator("body")).toContainText("Aborted by a user.");
});

test("should abort build when a user asks from the build page and run on_aborted tasks", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
desc: Abort test
tasks:
  - name: Wait for test
    run: ${gate.command}
on_aborted:
  - name: Aborted
    run: echo BINGO
`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "running");
    await wake.livePage(`/build/${id}`);
    await page.getByTestId("abort-build-button").click();
    await expect(page.locator("[data-cy-status]").first()).toHaveAttribute("data-cy-status", "aborted");
    await expect(page.locator("[data-cy-status]")).toHaveCount(3);
    await page.getByTestId("reload").nth(0).click();
    await page.getByTestId("reload").nth(1).click();
    await expect(page.locator("body")).toContainText("Aborted by a user.");
    await expect(page.locator("body")).toContainText("BINGO");
});

test("should abort build automatically when timeout is reached and run on_aborted tasks", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
desc: Timeout test
tasks:
  - name: Wait for timeout
    run: ${gate.command}
on_aborted:
  - name: Aborted
    run: echo BONGO
timeout: 1s
`);
    const id = await wake.runJob(name);
    await wake.showFeed(name);
    await expect(page.locator("[data-cy-status]")).toHaveAttribute("data-cy-status", "timed out");
    await page.getByTestId("open-build-button").click();
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy-status]")).toHaveCount(3);
    await expect(page.locator("[data-cy-status]").first()).toHaveAttribute("data-cy-status", "timed out");
    await page.getByTestId("reload").nth(0).click();
    await page.getByTestId("reload").nth(1).click();
    await expect(page.locator("body")).toContainText("BONGO");
});

test("should time out a build while a task checks its condition", async ({ page, wake }) => {
    const name = await wake.createJob(`
desc: Timeout in a condition
tasks:
  - name: First
    run: "true"
  - name: Wait in the condition
    if: sleep 30
    run: echo SHOULD-NOT-RUN
on_aborted:
  - name: Aborted
    run: echo BONGO
timeout: 1s
`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "timed out");
    await wake.livePage(`/build/${id}`);
    await expect(page.locator("[data-cy-status]").first()).toHaveAttribute("data-cy-status", "timed out");
    await expect(page.locator("[data-cy-status]")).toHaveCount(4);
    await expect(page.locator("[data-cy-status]").nth(2)).toHaveAttribute("data-cy-status", "timed out");
    await page.getByTestId("reload").nth(1).click();
    await page.getByTestId("reload").nth(2).click();
    await expect(page.locator("body")).toContainText("> Timed out.");
    await expect(page.locator("body")).toContainText("BONGO");
    await expect(page.locator("body")).not.toContainText("SHOULD-NOT-RUN");
});
