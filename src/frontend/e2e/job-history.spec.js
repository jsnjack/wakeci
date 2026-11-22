import { test, expect } from "./fixtures.js";

// B7: the job's last builds next to the title, and keys to the ones around
test("the build page shows the job's last builds and moves between them", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    const ids = [];
    for (let i = 0; i < 3; i++) {
        const id = await wake.runJob(name);
        await wake.waitForBuild(id);
        ids.push(id);
    }
    const [first, middle, last] = ids;
    await wake.openBuild(middle);
    const strip = page.getByTestId("job-history").getByTestId("outcome-strip");
    await expect(strip.locator("[data-cy-status]")).toHaveCount(3);
    await expect(strip.getByTestId("strip-current")).toHaveCount(1);

    await page.keyboard.press(",");
    await expect(page).toHaveURL(`/build/${first}`);
    await expect(page.getByTestId("page-title")).toContainText(`#${first}`);
    // Nothing older
    await page.keyboard.press(",");
    await expect(page).toHaveURL(`/build/${first}`);
    await page.keyboard.press(".");
    await expect(page).toHaveURL(`/build/${middle}`);
    await page.keyboard.press(".");
    await expect(page).toHaveURL(`/build/${last}`);
    // The same with the buttons
    const history = page.getByTestId("job-history");
    await expect(history.getByTestId("job-newer")).toBeDisabled();
    await history.getByTestId("job-older").click();
    await expect(page).toHaveURL(`/build/${middle}`);
});
