import { randomUUID } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { test, expect, login } from "./fixtures.js";

// A build artifact is a page a job wrote, opened by someone logged in to
// wakeci, on wakeci's origin. These tests open hostile ones in a browser
// session of their own, since a successful attack would change that session.
test.use({ authenticated: false });

// Builds a job whose artifacts are the given files, and returns the URL
// that serves their index.html (the file server redirects index.html there)
async function publishArtifacts(wake, files) {
    for (const [name, content] of Object.entries(files)) {
        await writeFile(join(wake.dir, name), content);
    }
    const copy = Object.keys(files)
        .map((name) => `'${join(wake.dir, name)}'`)
        .join(" ");
    const name = await wake.createJob(`
desc: Hostile artifact
tasks:
  - name: Write the report
    run: cp ${copy} .
artifacts:
  - "*"
`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id);
    return `/storage/build/${id}/artifacts/`;
}

// Creates an API key named name with the session of the page it runs in
function createKey(name) {
    return fetch("/api/keys/", {
        method: "POST",
        body: new URLSearchParams({ name, scope: "read", allJobs: "true", expiresInDays: "0" }),
    }).then((response) => response.status);
}

// Every way we know for a page to act as its viewer: send a request itself,
// or reach another page on the origin and send it from there. Each would
// create an API key named after it.
function hostileReport(prefix) {
    return `<!doctype html>
<meta charset="utf-8">
<title>report</title>
<img src="/auth/logout" alt="">
<pre id="results"></pre>
<script>
const createKey = (win, vector) =>
    win.fetch("/api/keys/", {
        method: "POST",
        body: new URLSearchParams({ name: "${prefix}-" + vector, scope: "read", allJobs: "true", expiresInDays: "0" }),
    }).then((response) => response.status);
const loaded = (element) => new Promise((resolve) => {
    element.onload = resolve;
    element.onerror = resolve;
    setTimeout(resolve, 1000);
});
const attempts = {
    fetch: () => createKey(window, "fetch"),
    beacon: () => navigator.sendBeacon("/api/keys/", new URLSearchParams({
        name: "${prefix}-beacon", scope: "read", allJobs: "true", expiresInDays: "0",
    })),
    iframe: async () => {
        const frame = document.createElement("iframe");
        frame.src = "/";
        document.body.append(frame);
        await loaded(frame);
        return createKey(frame.contentWindow, "iframe");
    },
    object: async () => {
        const object = document.createElement("object");
        object.type = "text/html";
        object.data = "/";
        document.body.append(object);
        await loaded(object);
        return createKey(object.contentWindow || object.contentDocument.defaultView, "object");
    },
    popup: async () => {
        const popup = window.open("/");
        await loaded(popup);
        return createKey(popup, "popup");
    },
    serviceWorker: () => navigator.serviceWorker.register("sw.js"),
};
(async () => {
    const results = {};
    for (const [vector, attempt] of Object.entries(attempts)) {
        try {
            // A request from a blocked frame's window may never settle
            const timeout = new Promise((resolve, reject) => setTimeout(() => reject(new Error("timed out")), 2000));
            results[vector] = "done: " + (await Promise.race([attempt(), timeout]));
        } catch (error) {
            results[vector] = "failed: " + error;
        }
    }
    document.getElementById("results").textContent = JSON.stringify(results, null, 2);
    document.body.dataset.done = "true";
})();
</script>
`;
}

// A page of its own: Playwright waits for the navigation that a blocked form
// starts, so the page can't be used after it
function hostileForm(prefix) {
    return `<!doctype html>
<meta charset="utf-8">
<title>form</title>
<form method="post" action="/api/keys/">
    <input name="name" value="${prefix}-form">
    <input name="scope" value="read">
    <input name="allJobs" value="true">
    <input name="expiresInDays" value="0">
</form>
<script>
addEventListener("load", () => setTimeout(() => document.forms[0].submit(), 100));
</script>
`;
}

test("a hostile artifact can't act as its viewer", async ({ page, backend, wake }) => {
    // Attempts that are blocked may only end by timing out
    test.setTimeout(60000);
    const prefix = `hostile-${randomUUID()}`;
    const keysNamed = async () => {
        const response = await backend.api.get("/api/keys/");
        await expect(response).toBeOK();
        return (await response.json()).keys.filter((key) => key.name.startsWith(prefix));
    };
    try {
        const report = await publishArtifacts(wake, {
            "index.html": hostileReport(prefix),
            "form.html": hostileForm(prefix),
            // A service worker could answer the artifact's requests with a
            // page that may send requests
            "sw.js": "self.addEventListener('fetch', () => {});",
        });
        await page.goto("/login");
        await login(page);

        // The same request from the app works, so a refused one in the
        // artifact was refused for where it came from
        expect(await page.evaluate(createKey, `${prefix}-control`)).toBe(200);

        await page.goto(report);
        await expect(page.locator("body")).toHaveAttribute("data-done", "true", { timeout: 30000 });
        await page.waitForLoadState("networkidle");
        const results = JSON.parse(await page.locator("#results").textContent());
        test.info().annotations.push({ type: "results", description: JSON.stringify(results) });
        expect(results.fetch).toMatch(/^failed/);
        expect(results.popup).toMatch(/^failed/);
        expect(results.serviceWorker).toMatch(/^failed/);
        expect(page.url()).toBe(`${wake.url}${report}`);

        const formBlocked = page.waitForEvent("console", (message) => message.text().includes("form-action"));
        await page.goto(`${report}form.html`);
        await formBlocked;

        expect((await keysNamed()).map((key) => key.name)).toEqual([`${prefix}-control`]);
        // The image didn't log the viewer out
        await expect(await page.request.get("/auth/_isLoggedIn")).toBeOK();
        await page.goto("/");
        expect(await page.evaluate(() => navigator.serviceWorker.getRegistrations().then((found) => found.length))).toBe(0);
    } finally {
        for (const key of await keysNamed()) {
            await expect(await backend.api.delete(`/api/keys/${key.id}`)).toBeOK();
        }
    }
});

// An artifact can set a "session" cookie for /api, which the browser sends
// before the session cookie. Over HTTPS the session cookie is
// __Host-session, which wakeci reads first.
test("an artifact can't replace the session over HTTPS", async ({ page, wake }) => {
    const report = await publishArtifacts(wake, {
        "index.html": `<!doctype html>
<meta charset="utf-8">
<title>report</title>
<script>
document.cookie = "session=planted; path=/api";
document.title = "planted";
</script>
`,
    });
    // As a TLS proxy in front of wakeci tells it; Chrome keeps Secure
    // cookies from 127.0.0.1
    await page.setExtraHTTPHeaders({ "X-Forwarded-Proto": "https" });
    await page.goto("/login");
    await login(page);
    expect((await page.context().cookies()).map((cookie) => cookie.name)).toEqual(["__Host-session"]);

    await page.goto(report);
    await expect(page).toHaveTitle("planted");
    expect((await page.context().cookies()).map((cookie) => `${cookie.name} ${cookie.path}`).sort()).toEqual(["__Host-session /", "session /api"]);

    await page.goto("/");
    expect(await page.evaluate(() => fetch("/api/whoami").then((response) => response.status))).toBe(200);
});
