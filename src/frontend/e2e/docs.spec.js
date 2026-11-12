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

test("should show the REST API docs in the page, in the app's theme", async ({ page }) => {
    await page.goto("/help");
    await page.getByTestId("help-tab-rest").click();
    await expect(page).toHaveURL(/\/help#rest$/);
    const docs = page.frameLocator("[data-cy=api-docs-frame]").locator("body");
    await expect(docs).toContainText("wakeci API documentation");
    // The docs take the app's colours, and change with its theme
    const theme = async () => ({
        dark: await page.evaluate(() => document.body.classList.contains("dark")),
        surface: await page.evaluate(() => getComputedStyle(document.body).getPropertyValue("--surface").trim()),
        docs: await docs.evaluate((body) => getComputedStyle(body).getPropertyValue("--scalar-background-1").trim()),
    });
    const before = await theme();
    expect(before.docs).toBe(before.surface);
    await expect(docs).toHaveClass(before.dark ? /dark-mode/ : /light-mode/);
    await page.locator("button:has(i:text-is('dark_mode'))").click();
    await expect(docs).toHaveClass(before.dark ? /light-mode/ : /dark-mode/);
    const after = await theme();
    expect(after.dark).toBe(!before.dark);
    expect(after.docs).toBe(after.surface);
    expect(after.docs).not.toBe(before.docs);
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

test("should serve the REST API as Markdown without credentials", async ({ backend, playwright }) => {
    const anonymous = await playwright.request.newContext({ baseURL: backend.url, storageState: { cookies: [], origins: [] } });
    const full = await anonymous.get("/llms-full.txt");
    expect(full.status()).toBe(200);
    expect(full.headers()["content-type"]).toContain("text/markdown");
    const text = await full.text();
    expect(text).toContain("### GET /api/feed");
    expect(text).toContain("### POST /api/job/{name}/run");
    expect(await (await anonymous.get("/docs/api.md")).text()).toBe(text);
    expect(await (await anonymous.get("/llms.txt")).text()).toContain("/llms-full.txt");
    await anonymous.dispose();
});
