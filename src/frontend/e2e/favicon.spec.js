import { test, expect } from "./fixtures.js";

// L4: a build's tab shows its status in the favicon
test("the build page puts its status on the favicon, other pages the plain one", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: hello\n    run: echo hello\n");
    const id = await wake.runJob(name);
    // The browser must load the drawn icon too, which the page's CSP decides
    const refused = [];
    page.on("console", (message) => message.text().includes("Content Security Policy") && refused.push(message.text()));
    await wake.openBuild(id);
    const icon = page.locator("link[rel='icon']");
    await expect(icon).toHaveAttribute("data-status", "finished");
    await expect(icon).toHaveAttribute("href", /^data:image\/png/);
    await page.waitForTimeout(200);
    expect(refused).toEqual([]);
    await page.getByTestId("nav-jobs").click();
    await expect(page).toHaveURL(/\/jobs/);
    await expect(icon).not.toHaveAttribute("data-status");
    await expect(icon).toHaveAttribute("href", "/favicon.ico");
});
