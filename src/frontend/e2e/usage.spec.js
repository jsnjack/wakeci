import { randomUUID } from "node:crypto";
import { test, expect } from "./fixtures.js";

// newKey makes a read key and a request context that sends it, without the
// worker's session
async function newKey(backend, playwright) {
    const name = `usage ${randomUUID()}`;
    const response = await backend.api.post("/api/keys/", {
        data: new URLSearchParams({ name, scope: "read", allJobs: "true", expiresInDays: "1" }).toString(),
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
    });
    expect(response.status(), await response.text()).toBe(200);
    const { key, token } = await response.json();
    const api = await playwright.request.newContext({
        baseURL: backend.url,
        storageState: { cookies: [], origins: [] },
        extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
    return { key, name, api };
}

// callTool calls an MCP tool the way a client does, in one request to /mcp
async function callTool(api, name, args = {}) {
    const response = await api.post("/mcp", {
        data: { jsonrpc: "2.0", id: 1, method: "tools/call", params: { name, arguments: args } },
        headers: { Accept: "application/json, text/event-stream" },
    });
    expect(response.status(), await response.text()).toBe(200);
    return response.json();
}

test("API usage shows a key's requests and MCP tool calls by route and tool", async ({ page, backend, playwright }) => {
    const { key, name, api } = await newKey(backend, playwright);
    expect((await api.get("/api/jobs/")).status()).toBe(200);
    expect((await api.get("/api/jobs/")).status()).toBe(200);
    expect((await api.get("/api/build/999999")).status()).toBe(404);
    expect((await api.get("/api/settings")).status()).toBe(403);
    const whoami = await callTool(api, "whoami");
    expect(whoami.result?.isError, JSON.stringify(whoami)).toBeFalsy();
    const refused = await callTool(api, "build_summary", { id: 999999 });
    expect(refused.result?.isError, JSON.stringify(refused)).toBe(true);
    await api.dispose();

    // Other tests' requests share the server, so the page shows only the key's
    await page.goto(`/analytics/usage?who=${encodeURIComponent(`apikey:${key.id}`)}`);
    await expect(page.getByTestId("analytics-usage-tab")).toHaveClass(/active/);
    await expect(page.getByTestId("tile-requests").getByTestId("tile-value")).toHaveText("6");
    await expect(page.getByTestId("tile-serverErrors").getByTestId("tile-value")).toHaveText("0");
    await expect(page.getByTestId("tile-toolCalls").getByTestId("tile-value")).toHaveText("2");
    await expect(page.getByTestId("tile-toolCalls")).toContainText("1 failed");

    const routes = page.getByTestId("routes-row");
    await expect(routes).toHaveCount(4);
    await expect(routes.filter({ hasText: "GET /api/jobs/" })).toContainText("API with key");
    await expect(routes.filter({ hasText: "GET /api/jobs/" }).locator("td").nth(2)).toHaveText("2");
    await expect(routes.filter({ hasText: "GET /api/build/{id}" }).locator("td").nth(5)).toHaveText("1");
    // A route keys can't use is refused without naming it
    await expect(routes.filter({ hasText: "No route" }).locator("td").nth(4)).toHaveText("1");
    await expect(routes.filter({ hasText: "POST /mcp" })).toContainText("MCP");

    const tools = page.getByTestId("tools-row");
    await expect(tools).toHaveCount(2);
    await expect(tools.filter({ hasText: "whoami" }).locator("td").nth(2)).toHaveText("100%");
    await expect(tools.filter({ hasText: "build_summary" }).locator("td").nth(4)).toHaveText("1");

    await expect(page.getByTestId("people")).toContainText(name);
    await expect(page.getByTestId("interface")).toHaveCount(2);

    // The routes table sorts and filters
    await page.getByTestId("routes-search").fill("mcp");
    await expect(routes).toHaveCount(1);
    await page.getByTestId("routes-search").fill("");
    await page.getByTestId("routes").getByRole("columnheader", { name: "Client errors" }).click();
    await expect(routes.first()).toContainText("GET /api/build/{id}");

    // The interface filter keeps the address and every way in
    await page.getByTestId("via-filter").click();
    await page.getByTestId("via-filter").fill("MCP");
    await page.getByTestId("via-filter").press("Enter");
    await expect(page).toHaveURL(/via=mcp/);
    await expect(page.getByTestId("tile-requests").getByTestId("tile-value")).toHaveText("2");
    await expect(page.getByTestId("interface")).toHaveCount(2);

    await page.getByTestId("analytics-builds-tab").click();
    await expect(page).toHaveURL("/analytics");
    await expect(page.getByTestId("analytics-builds-tab")).toHaveClass(/active/);
});
