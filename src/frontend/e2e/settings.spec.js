import { test, expect, login } from "./fixtures.js";

test.use({ authenticated: false });

test("should modify settings", async ({ page }) => {
    await page.goto("/settings");
    await login(page);
    await expect(page).toHaveURL(/\/settings$/);
    await page.locator("input[type=password]").fill("secret");
    await page.locator("#concurrent-builds").fill("100");
    await page.locator("#build-history-size").fill("100");
    await page.getByTestId("save-settings").click();
    await expect(page.locator(".notification-content")).toContainText("Saved");
    await page.getByTestId("logout").click();
    await expect(page).toHaveURL(/\/login/);
    await login(page, "secret");
    await page.goto("/settings");
    await expect(page.locator("#concurrent-builds")).toHaveValue("100");
    await expect(page.locator("#build-history-size")).toHaveValue("100");
    await page.locator("input[type=password]").fill("admin");
    await page.locator("#concurrent-builds").fill("2");
    await page.locator("#build-history-size").fill("200");
    await page.getByTestId("save-settings").click();
    await expect(page.locator(".notification-content")).toContainText("Saved");
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
