import { test, expect } from "./fixtures.js";

test("should open API docs", async ({ page }) => {
    await page.goto("/docs/api/");
    await expect(page.locator("body")).toContainText("wakeci API documentation");
});

test("should open syntax docs", async ({ page }) => {
    await page.goto("/help");
    await expect(page.locator("body")).toContainText("Ask a cow to say something smart");
});
