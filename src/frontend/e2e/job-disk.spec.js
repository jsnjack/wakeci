import { test, expect } from "./fixtures.js";

// disk.go: a build's size is measured once it ends, and the job's insights
// add up its builds in the history
test("job insights show what the job's builds keep on disk", async ({ page, wake }) => {
    const name = await wake.createJob(
        "artifacts:\n  - report/**\ntasks:\n  - name: report\n    run: mkdir -p report && head -c 300000 /dev/zero > report/data.bin && echo done\n",
    );
    await wake.waitForBuild(await wake.runJob(name));
    // Measured in the background once the build left the queue
    await expect.poll(async () => (await (await page.request.get(`/api/job/${encodeURIComponent(name)}/insights`)).json()).disk.builds).toBe(1);
    await page.goto(`/job/${encodeURIComponent(name)}/insights`);
    const tile = page.getByTestId("insight-disk");
    await expect(tile.getByTestId("tile-value")).toHaveText(/^\d+(\.\d+)? (kB|MB)$/);
    await expect(tile).toContainText("artifacts 292.97 kB");
    await expect(tile).toContainText("1 build");
});
