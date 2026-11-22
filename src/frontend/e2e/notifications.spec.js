import { test, expect } from "./fixtures.js";

// L5: notifications follow the user to every page and survive a reload.
// The browser has no permission for system notifications here, so they
// show as messages on the page.
test("the bell of a build tells on another page, after a reload, when it ends", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`tasks:\n  - name: wait\n    run: ${gate.command}\n`);
    const id = await wake.runJob(name);
    await wake.openBuild(id, "running");
    await page.getByTestId("subscribe-button").click();
    await expect(page.getByTestId("subscribe-button")).toHaveAttribute("aria-pressed", "true");
    await page.reload();
    await expect(page.getByTestId("subscribe-button")).toHaveAttribute("aria-pressed", "true");

    await wake.livePage("/keys");
    await gate.release();
    const message = page.locator(".toast", { hasText: `${name} finished` });
    await expect(message).toBeVisible();
    await expect(message.getByRole("link", { name: `#${id}` })).toHaveAttribute("href", `/build/${id}/`);
});

test("a watched search tells when a matching build ends, on any page", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: fail\n    run: exit 1\n");
    const other = await wake.createJob("tasks:\n  - name: pass\n    run: echo ok\n");
    await wake.showFeed(name);
    await page.getByTestId("watch-search").click();
    await expect(page.getByTestId("watch-search")).toHaveAttribute("aria-pressed", "true");

    await wake.livePage("/jobs");
    await wake.waitForBuild(await wake.runJob(other));
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "failed");
    await expect(page.locator(".toast", { hasText: `${name} failed` })).toBeVisible();
    await expect(page.locator(".toast", { hasText: other })).toHaveCount(0);

    // Off again, from the same search
    await wake.showFeed(name);
    await page.getByTestId("watch-search").click();
    await expect(page.getByTestId("watch-search")).toHaveAttribute("aria-pressed", "false");
});
