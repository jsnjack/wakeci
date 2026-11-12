import { readFile } from "node:fs/promises";
import { test, expect } from "./fixtures.js";

const form = (fields) => ({ data: new URLSearchParams(fields).toString(), headers: { "Content-Type": "application/x-www-form-urlencoded" } });

test("should show what Wakefile.yaml set, read-only", async ({ page, backend }) => {
    await page.goto("/settings");
    const card = page.getByTestId("server-card");
    await expect(card.getByTestId("server-port")).toHaveText(new URL(backend.url).port);
    await expect(card.getByTestId("server-config-file")).toContainText("Wakefile.yaml");
    await expect(card.getByTestId("server-jobdir")).toContainText("/");
    await expect(card.getByTestId("server-sso")).toContainText("Not set up");
});

test("should warn before the identity provider's certificate expires", async ({ page }) => {
    const soon = new Date(Date.now() + 10 * 86400000).toISOString();
    await page.route("**/api/settings", async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        json.server.sso = {
            idpMetadata: "/etc/wakeci/idp.xml",
            acsUrl: "https://ci.example.com/auth/sso/acs",
            entityId: "https://ci.example.com",
            certificates: [{ subject: "CN=Google", notAfter: soon }],
        };
        await route.fulfill({ response, json });
    });
    await page.goto("/settings");
    const card = page.getByTestId("server-card");
    await expect(card.getByTestId("server-entity")).toHaveText("https://ci.example.com");
    await expect(card.getByTestId("server-cert-warning")).toContainText(/Expires in (9|10) days/);
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
