import { test, expect } from "./fixtures.js";

// L1: the rows the user reads don't move
test("new builds wait behind a pill while the user has scrolled down", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    for (let i = 0; i < 10; i++) {
        await wake.waitForBuild(await wake.runJob(name));
    }
    await page.setViewportSize({ width: 1024, height: 400 });
    await wake.showFeed(name);
    await expect(page.locator(".feed-row")).toHaveCount(10);
    await page.mouse.move(0, 0);
    // The feed learns of a scroll from the event, a frame after scrollTo;
    // a build started before it would come in at once
    await page.evaluate(
        () =>
            new Promise((resolve) => {
                window.addEventListener("scroll", resolve, { once: true });
                window.scrollTo(0, 400);
            }),
    );
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(120);

    const id = await wake.runJob(name);
    await expect(page.getByTestId("new-builds")).toHaveText(/1 new build/);
    await expect(page.locator(`.feed-row[data-cy-build="${id}"]`)).toHaveCount(0);
    // It stays in sight while the user reads further down
    await expect(page.getByTestId("new-builds")).toBeInViewport();
    await page.getByTestId("new-builds").click();
    await expect(page.locator(`.feed-row[data-cy-build="${id}"]`)).toBeVisible();
    await expect(page.getByTestId("new-builds")).toHaveCount(0);
});

test("new builds come in at once while the user is at the top", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    await wake.showFeed(name);
    await page.mouse.move(0, 0);
    const id = await wake.runJob(name);
    await expect(page.locator(`.feed-row[data-cy-build="${id}"]`)).toBeVisible();
    await expect(page.getByTestId("new-builds")).toHaveCount(0);
});
