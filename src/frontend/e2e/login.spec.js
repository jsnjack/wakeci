import { test, expect, login } from "./fixtures.js";

test.use({ authenticated: false });

test("should not offer single sign-on when it isn't configured", async ({ page }) => {
    const methods = page.waitForResponse("**/auth/methods");
    await page.goto("/login");
    expect((await (await methods).json()).sso).toBe(false);
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.getByTestId("sso-login")).toHaveCount(0);
});

test("should offer single sign-on first and the password only when asked", async ({ page }) => {
    await page.route("**/auth/methods", (route) => route.fulfill({ json: { sso: true } }));
    await page.goto("/login?redirect=/jobs");
    await expect(page.getByTestId("sso-login")).toHaveAttribute("href", "/auth/sso/login?redirect=%2Fjobs");
    // No password field in the page, so password managers have nothing to fill
    await expect(page.locator("input[type=password]")).toHaveCount(0);

    await page.getByTestId("admin-login").click();
    await expect(page.locator("#password")).toBeFocused();
    await expect(page.getByTestId("admin-login")).toHaveCount(0);
    await login(page);
    await expect(page).toHaveURL(/\/jobs$/);
});

test("should not show the password field before it knows about single sign-on", async ({ page }) => {
    let answer;
    const answered = new Promise((resolve) => (answer = resolve));
    await page.route("**/auth/methods", async (route) => {
        await answered;
        await route.fulfill({ json: { sso: true } });
    });
    await page.goto("/login");
    await expect(page.locator("progress")).toBeVisible();
    await expect(page.locator("input[type=password]")).toHaveCount(0);
    answer();
    await expect(page.getByTestId("sso-login")).toBeVisible();
    await expect(page.locator("input[type=password]")).toHaveCount(0);
});

test("should explain a rejected account and keep the redirect", async ({ page }) => {
    await page.route("**/auth/methods", (route) => route.fulfill({ json: { sso: true } }));
    await page.goto("/login?error=sso_not_allowed&redirect=/jobs");
    await expect(page.locator(".notification-content")).toContainText("not in a group that is allowed to log in");
    await expect(page).not.toHaveURL(/error=/);
    await expect(page.getByTestId("sso-login")).toHaveAttribute("href", "/auth/sso/login?redirect=%2Fjobs");
});

test("should ignore unknown error codes", async ({ page }) => {
    await page.goto("/login?error=constructor");
    await expect(page.locator("#password")).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.locator(".notification-content")).toHaveCount(0);
});

test("should only redirect login to internal paths", async ({ page }) => {
    await page.goto("/login?redirect=" + encodeURIComponent("https://attacker.example/path"));
    await login(page);
    await expect(page).toHaveURL(/\/$/);
    await page.goto("/login?redirect=" + encodeURIComponent("/jobs"));
    await expect(page).toHaveURL(/\/jobs$/);
});

test("should reset websocket failures after logging in again", async ({ page }) => {
    await page.addInitScript(() => {
        const browserSetTimeout = window.setTimeout.bind(window);
        window.setTimeout = (callback, delay, ...args) => browserSetTimeout(callback, delay === 1000 ? 10 : delay, ...args);
        window.__wsAttempts = 0;
        window.WebSocket = class FakeWebSocket {
            constructor() {
                this.listeners = {};
                window.__wsAttempts += 1;
                if (window.__wsAttempts <= 9) {
                    browserSetTimeout(() => this.emit("close", {}), 0);
                } else {
                    window.__pendingWebSocket = this;
                }
            }
            addEventListener(type, listener) {
                this.listeners[type] = listener;
            }
            emit(type, event) {
                this.listeners[type]?.(event);
            }
            send() {}
        };
    });
    await page.goto("/");
    await login(page);
    await expect.poll(() => page.evaluate(() => window.__wsAttempts)).toBe(10);
    await page.getByTestId("logout").click();
    await expect(page).toHaveURL(/\/login/);
    await login(page);
    await expect(page).toHaveURL(/\/$/);
    const loginRedirects = [];
    page.on("framenavigated", (frame) => {
        if (frame === page.mainFrame() && new URL(frame.url()).pathname === "/login") {
            loginRedirects.push(frame.url());
        }
    });
    const attempts = await page.evaluate(() => window.__wsAttempts);
    await page.evaluate(() => window.__pendingWebSocket.emit("close", {}));
    await expect.poll(() => page.evaluate(() => window.__wsAttempts)).toBe(attempts + 1);
    expect(loginRedirects).toEqual([]);
    await expect(page).toHaveURL(/\/$/);
});
