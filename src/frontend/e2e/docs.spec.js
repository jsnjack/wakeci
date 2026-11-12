import { test, expect } from "./fixtures.js";

test("should open API docs", async ({ page }) => {
    await page.goto("/docs/api/");
    await expect(page.locator("body")).toContainText("wakeci API documentation");
});

test("should open syntax docs", async ({ page }) => {
    await page.goto("/help");
    await expect(page.locator("body")).toContainText("Ask a cow to say something smart");
});

test("should describe the MCP tools from the server", async ({ page, wake }) => {
    await page.goto("/help#mcp");
    await expect(page.getByTestId("mcp-connect")).toContainText(`claude mcp add --transport http wakeci ${wake.url}/mcp`);
    const summary = page.getByTestId("mcp-tool-build_summary");
    await expect(summary).toContainText("id");
    await expect(summary).toContainText("integer, required");
    await expect(summary.getByTestId("mcp-tool-scope")).toHaveText("read");
    await expect(page.getByTestId("mcp-tool-run_job").getByTestId("mcp-tool-scope")).toHaveText("run");
    await expect(page.getByTestId("mcp-tools").locator("tbody tr")).toHaveCount(8);
});

test("should show the REST API docs in the page", async ({ page }) => {
    await page.goto("/help");
    await page.getByTestId("help-tab-rest").click();
    await expect(page).toHaveURL(/\/help#rest$/);
    await expect(page.frameLocator("[data-cy=api-docs-frame]").locator("body")).toContainText("wakeci API documentation");
});
