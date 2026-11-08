import { test, expect } from "./fixtures.js";

test("should show a planned restart", async ({ page }) => {
    const abortAt = new Date(Date.now() + 30 * 60 * 1000);
    await page.route("**/api/queue/", (route) =>
        route.fulfill({
            json: { stopping: true, abortAt: abortAt.toISOString(), restartVersion: "9.9.9", running: 1, queued: 2 },
        }),
    );
    await page.goto("/");
    const banner = page.getByTestId("queue-stopping");
    await expect(banner).toContainText("restarts into version 9.9.9");
    await expect(banner).toContainText("New builds wait in the queue");
});

test("should offer an available update", async ({ page }) => {
    const notes = "https://github.com/jsnjack/wakeci/releases/tag/v9.9.9";
    await page.route("**/api/update/", (route) =>
        route.request().method() === "GET"
            ? route.fulfill({ json: { current: "1.0.0", latest: "9.9.9", available: true, releaseNotes: notes } })
            : route.fulfill({ contentType: "text/plain", body: "Installed 9.9.9. wakeci restarts once the running builds end." }),
    );
    await page.goto("/settings");
    await expect(page.getByTestId("version")).toHaveText("wakeci 1.0.0");
    await expect(page.getByTestId("update-status")).toContainText("Version 9.9.9 is available.");
    await expect(page.getByTestId("release-notes")).toHaveAttribute("href", notes);
    await expect(page.getByTestId("update-install")).toHaveText("Update to 9.9.9");

    await page.getByTestId("update-install").click();
    const [request] = await Promise.all([
        page.waitForRequest((r) => r.url().endsWith("/api/update/") && r.method() === "POST"),
        page.getByTestId("update-confirm").click(),
    ]);
    expect(request.postData()).toContain("9.9.9");
    await expect(page.locator(".notification-content")).toContainText("Installed 9.9.9");
});

// The Update button stays in place, disabled, when there is nothing to install
for (const [name, answer, status] of [
    ["say when wakeci is up to date", { current: "1.0.0", latest: "1.0.0", available: false, checkedAt: new Date().toISOString() }, "Up to date · checked at"],
    [
        "say why wakeci can't update itself",
        { current: "1.0.0", latest: "9.9.9", available: true, problem: "wakeci can't write to /usr/bin, the directory of its binary." },
        "Version 9.9.9 is available, but wakeci can't update itself: wakeci can't write to /usr/bin",
    ],
    [
        "say why the check failed",
        { current: "1.0.0", error: "get latest release: GitHub answered 403 Forbidden" },
        "Couldn't check for updates: get latest release",
    ],
]) {
    test(`should ${name}`, async ({ page }) => {
        await page.route("**/api/update/", (route) => route.fulfill({ json: answer }));
        await page.goto("/settings");
        await expect(page.getByTestId("update-status")).toContainText(status);
        await expect(page.getByTestId("update-install")).toHaveText("Update");
        await expect(page.getByTestId("update-install")).toBeDisabled();
        await expect(page.locator(".notification-content")).toHaveCount(0);
    });
}

test("should show a planned restart on the update card", async ({ page }) => {
    await page.route("**/api/queue/", (route) =>
        route.fulfill({ json: { stopping: true, abortAt: new Date(Date.now() + 60000).toISOString(), restartVersion: "9.9.9" } }),
    );
    await page.route("**/api/update/", (route) => route.fulfill({ json: { current: "1.0.0", latest: "9.9.9", available: true } }));
    await page.goto("/settings");
    await expect(page.getByTestId("update-status")).toHaveText("Restarting into 9.9.9 once the running builds end.");
    await expect(page.getByTestId("update-install")).toBeDisabled();
});

test("should reload when the server runs a new version", async ({ page }) => {
    let version = "1.0.0";
    await page.route("**/api/queue/", (route) => route.fulfill({ json: { stopping: false, running: 0, queued: 0, version } }));
    let socket;
    await page.routeWebSocket(/\/ws$/, (ws) => {
        ws.connectToServer();
        socket = ws;
    });
    const fetched = page.waitForResponse((response) => response.url().endsWith("/api/queue/"));
    await page.goto("/");
    await fetched;
    await expect(page.getByTestId("logout")).toBeVisible();

    // The restart after an update closes the connection, and the new server
    // answers with its version
    version = "2.0.0";
    const reloaded = page.waitForEvent("load");
    socket.close();
    await reloaded;
});
