import { readFile } from "node:fs/promises";
import { test, expect } from "./fixtures.js";

const form = (fields) => ({ data: new URLSearchParams(fields).toString(), headers: { "Content-Type": "application/x-www-form-urlencoded" } });

test("should show editable settings without the server configuration", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByLabel("Concurrent builds", { exact: true })).toBeVisible();
    await expect(page.getByLabel("Builds to keep", { exact: true })).toBeVisible();
    await expect(page.getByTestId("server-card")).toHaveCount(0);
    await expect(page.getByTestId("update-card")).toBeVisible();
});

test("should download the settings and import them back", async ({ page, backend }) => {
    const original = await (await backend.api.get("/api/settings")).json();
    try {
        await page.goto("/settings");
        const downloading = page.waitForEvent("download");
        await page.getByTestId("settings-download").click();
        const download = await downloading;
        expect(download.suggestedFilename()).toMatch(/^wakeci-settings-.*\.yaml$/);
        const path = await download.path();
        const text = await readFile(path, "utf8");
        expect(text).toContain(`concurrentBuilds: ${original.concurrentBuilds}`);
        expect(text).not.toContain("password:");

        // Changed after the download; the import brings the file's values back
        await expect(
            await backend.api.post("/api/settings", form({ concurrentBuilds: "7", buildHistorySize: String(original.buildHistorySize), apiKeyRate: "3" })),
        ).toBeOK();
        await page.reload();
        await expect(page.locator("#concurrent-builds")).toHaveValue("7");
        await page.getByTestId("settings-import-file").setInputFiles(path);
        await page.getByTestId("settings-import-confirm").click();
        await expect(page.locator(".notification-content")).toContainText("Imported");
        await expect(page.locator(".notification-content")).toContainText("concurrent builds");
        await expect(page.locator("#concurrent-builds")).toHaveValue(String(original.concurrentBuilds));
        await expect(page.getByTestId("api-key-rate")).toHaveValue(String(original.apiKeyRate));

        // A file with a mistake changes nothing
        await page
            .getByTestId("settings-import-file")
            .setInputFiles({ name: "bad.yaml", mimeType: "application/yaml", buffer: Buffer.from("concurrentBuilds: 0\n") });
        await page.getByTestId("settings-import-confirm").click();
        await expect(page.locator(".notification-content").last()).toContainText("concurrentBuilds must be");
        expect((await (await backend.api.get("/api/settings")).json()).concurrentBuilds).toBe(original.concurrentBuilds);
    } finally {
        // Other tests of this worker use the same wakeci
        await backend.api.post(
            "/api/settings",
            form({
                concurrentBuilds: String(original.concurrentBuilds),
                buildHistorySize: String(original.buildHistorySize),
                apiKeyRate: String(original.apiKeyRate),
                apiKeyBurst: String(original.apiKeyBurst),
            }),
        );
    }
});
