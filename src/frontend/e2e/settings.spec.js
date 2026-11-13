import { test, expect, login } from "./fixtures.js";

test.use({ authenticated: false });

test("should modify settings", async ({ page, backend }) => {
    await page.goto("/settings");
    await login(page);
    await expect(page).toHaveURL(/\/settings$/);
    await page.locator("input[type=password]").fill("secret");
    await page.locator("#concurrent-builds").fill("100");
    await page.locator("#build-history-size").fill("100");
    await expect(page.getByTestId("api-key-rate")).toHaveValue("10");
    await expect(page.getByTestId("api-key-burst")).toHaveValue("120");
    await page.getByTestId("api-key-rate").fill("50");
    await page.getByTestId("api-key-burst").fill("500");
    // The example follows the fields: 500 / 50 seconds to fill the burst
    await expect(page.getByTestId("api-key-example")).toHaveText(
        "With these values, a script can send 500 requests as fast as it can, even all within one second, then 50 a second. " +
            "The burst isn't reset on a timer: every second gives 50 of it back and every request uses one. " +
            "So 10 seconds without requests fill it again, and a key that keeps sending 50 a second never gets any back.",
    );
    await page.getByTestId("save-settings").click();
    await expect(page.locator(".notification-content")).toContainText("Saved");
    await page.getByTestId("logout").click();
    await expect(page).toHaveURL(/\/login/);
    await login(page, "secret");
    await page.goto("/settings");
    await expect(page.locator("#concurrent-builds")).toHaveValue("100");
    await expect(page.locator("#build-history-size")).toHaveValue("100");
    await expect(page.getByTestId("api-key-rate")).toHaveValue("50");
    await expect(page.getByTestId("api-key-burst")).toHaveValue("500");
    await page.locator("input[type=password]").fill("admin");
    await page.locator("#concurrent-builds").fill("2");
    await page.locator("#build-history-size").fill("200");
    await page.getByTestId("api-key-rate").fill("10");
    await page.getByTestId("api-key-burst").fill("120");
    await page.getByTestId("save-settings").click();
    await expect(page.locator(".notification-content")).toContainText("Saved");
    // The password change ended the other password sessions
    expect((await backend.api.get("/api/settings")).status()).toBe(401);
    await backend.logIn();
});

test("should not offer the password to SSO users", async ({ page }) => {
    await page.route("**/api/settings", (route) =>
        route.request().method() === "GET" ? route.fulfill({ json: { concurrentBuilds: 2, buildHistorySize: 200, admin: false } }) : route.continue(),
    );
    await page.goto("/settings");
    await login(page);
    await expect(page.locator("#concurrent-builds")).toBeVisible();
    await expect(page.locator("input[type=password]")).toHaveCount(0);
});

test("should limit each API key as the settings say", async ({ backend, playwright }) => {
    const form = (fields) => ({ data: new URLSearchParams(fields).toString(), headers: { "Content-Type": "application/x-www-form-urlencoded" } });
    const settings = await (await backend.api.get("/api/settings")).json();
    const save = async (rate, burst) => {
        const fields = { concurrentBuilds: settings.concurrentBuilds, buildHistorySize: settings.buildHistorySize, apiKeyRate: rate, apiKeyBurst: burst };
        await expect(await backend.api.post("/api/settings", form(fields))).toBeOK();
    };
    const created = await backend.api.post("/api/keys/", form({ name: "limited", scope: "read", allJobs: "true", expiresInDays: "1" }));
    await expect(created).toBeOK();
    const key = await playwright.request.newContext({
        baseURL: backend.url,
        storageState: { cookies: [], origins: [] },
        extraHTTPHeaders: { Authorization: `Bearer ${(await created.json()).token}` },
    });
    // Other tests of this worker use the same wakeci
    try {
        await save(1, 2);
        expect((await key.get("/api/whoami")).status()).toBe(200);
        expect((await key.get("/api/whoami")).status()).toBe(200);
        const refused = await key.get("/api/whoami");
        expect(refused.status()).toBe(429);
        expect(Number(refused.headers()["retry-after"])).toBeGreaterThanOrEqual(1);
    } finally {
        await save(10, 120);
        await key.dispose();
    }
});
