import { test, expect, openLog } from "./fixtures.js";

// Tips show on hover, inside the window and above anything that clips
async function tipOf(page, target) {
    await target.hover();
    const tip = page.getByTestId("tip");
    await expect(tip).toBeVisible();
    const box = await tip.boundingBox();
    const size = page.viewportSize();
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(size.width);
    expect(box.y + box.height).toBeLessThanOrEqual(size.height);
    return tip;
}

test("tips show on hover and stay inside the window, also in the log viewer", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    const id = await wake.runJob(name);
    await wake.openBuild(id);
    // At the window's right edge
    await expect(await tipOf(page, page.getByTestId("toggle-theme"))).toHaveText(/theme$/);
    await page.mouse.move(0, 0);
    await expect(page.getByTestId("tip")).toHaveCount(0);
    // In the log viewer's bar, which used to cut tips off
    await openLog(page);
    await expect(await tipOf(page, page.getByRole("button", { name: "Wrap long lines" }))).toHaveText("Wrap long lines");
    // A click that changes the tip changes the shown one
    const theme = page.getByTestId("toggle-theme");
    const before = await (await tipOf(page, theme)).textContent();
    await theme.click();
    await expect(page.getByTestId("tip")).not.toHaveText(before);
    await expect(page.getByTestId("tip")).toHaveText(/theme$/);
});
