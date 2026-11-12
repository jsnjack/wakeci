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

test("should give each MCP client its connect command with the key", async ({ page, context, wake }) => {
    const endpoint = `${wake.url}/mcp`;
    await page.goto("/help#mcp");
    await page.getByTestId("mcp-key").fill(" wk_id_secret ");
    const command = page.getByTestId("mcp-connect");
    await expect(command).toHaveText(`claude mcp add --transport http wakeci ${endpoint} \\\n  --header "Authorization: Bearer wk_id_secret"`);

    await page.getByTestId("mcp-client-codex").click();
    await expect(page.getByTestId("mcp-client-codex")).toHaveAttribute("aria-selected", "true");
    await expect(command).toHaveText(`export WAKECI_API_KEY=wk_id_secret\ncodex mcp add wakeci --url ${endpoint} --bearer-token-env-var WAKECI_API_KEY`);
    await expect(page.getByTestId("mcp-client-note")).toContainText("WAKECI_API_KEY");

    await page.getByTestId("mcp-client-copilot").click();
    await expect(command).toHaveText(`copilot mcp add --transport http wakeci ${endpoint} \\\n  --header "Authorization: Bearer wk_id_secret"`);
    await expect(page.getByTestId("mcp-vscode-install")).toBeHidden();

    await page.getByTestId("mcp-client-vscode").click();
    const server = { name: "wakeci", type: "http", url: endpoint, headers: { Authorization: "Bearer wk_id_secret" } };
    await expect(command).toHaveText(`code --add-mcp '${JSON.stringify(server)}'`);
    const install = await page.getByTestId("mcp-vscode-install").getAttribute("href");
    expect(install.startsWith("vscode:mcp/install?")).toBe(true);
    expect(JSON.parse(decodeURIComponent(install.slice("vscode:mcp/install?".length)))).toEqual(server);

    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.getByTestId("mcp-copy").click();
    await expect(page.getByTestId("mcp-copy")).toContainText("check");
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`code --add-mcp '${JSON.stringify(server)}'`);

    // Without a key the commands show where it goes
    await page.getByTestId("mcp-key").fill("");
    await expect(command).toContainText("Bearer wk_...");
});
