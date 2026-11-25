import { test, expect, openLog } from "./fixtures.js";

// The log viewer (B4) and the build page's search (docs/logs.md). The log
// repeats each command first, so the words the tests look for are split
// with quotes there: "FA""IL" prints FAIL.

test("should show the colours and links of task output", async ({ page, wake }) => {
    const name = await wake.createJob(`
color: true
tasks:
  - name: Colours
    run: printf '\\033[31mred\\033[0m and plain, see https://example.com/report.\\n'
`);
    const id = await wake.runJob(name);
    await wake.openBuild(id);
    await openLog(page);
    const log = page.getByTestId("task-log");
    await expect(log.locator(".ansi-fg-1")).toHaveText("red");
    // The output's link; the command the log repeats has one too
    await expect(log.locator("a.log-link").last()).toHaveAttribute("href", "https://example.com/report");
    await expect(log.locator("a.log-link").last()).toHaveAttribute("rel", "noopener noreferrer");
    // The raw log stays plain
    expect(await (await page.request.get(`/storage/build/${id}/task_0.log`)).text()).not.toContain("\x1b");
});

test("should search a log, and show only the matching lines", async ({ page, wake }) => {
    const name = await wake.createJob(`
tasks:
  - name: Talk
    run: for i in $(seq 1 30); do echo "line $i"; done; echo "FA""IL one"; echo "fa""il two"
`);
    const id = await wake.runJob(name);
    await wake.openBuild(id);
    await openLog(page);
    await page.getByTestId("log-search-open").click();
    await page.getByTestId("log-search").fill("fail");
    await expect(page.getByTestId("log-search-count")).toHaveText("1 of 2");
    await page.getByTestId("log-search").press("Enter");
    await expect(page.getByTestId("log-search-count")).toHaveText("2 of 2");
    await page.getByTestId("log-only-matching").check();
    await expect(page.getByTestId("log-search-count")).toHaveText(/^2 matching lines of \d+$/);
    await expect(page.getByTestId("task-log").locator(".log-gap").first()).toContainText("lines");
    await expect(page.getByTestId("task-log")).toContainText("fail two");
    await expect(page.getByTestId("task-log")).not.toContainText("line 15");
});

test("should find what failed with the Errors search", async ({ page, wake }) => {
    const name = await wake.createJob(`
tasks:
  - name: Test
    run: |
      for i in $(seq 1 30); do echo "ok $i, 0 errors"; done
      echo "--- FA""IL: TestLogin (0.01s)"
      echo "no failures here"
      echo "Trace""back (most recent call last):"
      echo "ValueE""rror: bad input"
      exit 3
`);
    const id = await wake.runJob(name);
    await wake.openBuild(id, "failed");
    await openLog(page);
    await page.getByTestId("log-errors").click();
    // FAIL, the traceback, ValueError and wakeci's "Exit code: 3"
    await expect(page.getByTestId("log-search-count")).toHaveText(/ of 4$/);
    await expect(page.getByTestId("log-errors")).toHaveAttribute("aria-pressed", "true");
    await page.getByTestId("log-only-matching").check();
    await expect(page.getByTestId("log-search-count")).toHaveText(/^4 matching lines of /);
    await expect(page.getByTestId("task-log").locator(".cm-line")).toHaveText([/--- FAIL: TestLogin/, /Traceback/, /ValueError: bad input/, /> Exit code: 3/]);
    // Typing searches the text instead
    await page.getByTestId("log-search").fill("ok 1");
    await expect(page.getByTestId("log-errors")).toHaveAttribute("aria-pressed", "false");
});

test("should put marked lines in the address and open them from it", async ({ page, wake }) => {
    const name = await wake.createJob(`
tasks:
  - name: Talk
    run: for i in $(seq 1 10); do echo "line $i"; done
`);
    const id = await wake.runJob(name);
    await wake.openBuild(id);
    await openLog(page);
    const gutter = page.getByTestId("task-log").locator(".cm-lineNumbers .cm-gutterElement");
    await gutter.filter({ hasText: /^3$/ }).click();
    await gutter.filter({ hasText: /^5$/ }).click({ modifiers: ["Shift"] });
    await expect(page).toHaveURL(new RegExp(`/build/${id}#t0-L3-L5$`));
    await page.reload();
    await expect(page.getByTestId("task-log").locator(".log-marked")).toHaveCount(3);
});

// A log over 50 MB opens at its tail, numbered from where the tail starts
// in the log (X-First-Line), so a line link opens that line
// On a full disk the live log numbers lines the file doesn't have: a read
// that still lacks them is the last, and live lines are taken as they come
test("should take live lines the log file lacks without reading it again and again", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`tasks:\n  - name: Talk\n    run: echo "n""=1."; ${gate.command}\n`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "running");
    let reads = 0;
    page.on("request", (request) => {
        if (new URL(request.url()).pathname.startsWith(`/api/build/${id}/log/`)) {
            reads++;
        }
    });
    let socket;
    let subscribed = false;
    await page.routeWebSocket(/\/ws$/, (ws) => {
        const server = ws.connectToServer();
        ws.onMessage((message) => {
            subscribed ||= message.includes(`"build:log:${id}"`);
            server.send(message);
        });
        server.onMessage((message) => ws.send(message));
        socket = ws;
    });
    await page.goto(`/build/${id}`);
    await page.getByTestId("log-search-open").click();
    await page.getByTestId("log-search").fill("n=");
    await expect(page.getByTestId("log-search-count")).toHaveText("1 of 1");
    await expect.poll(() => subscribed).toBe(true);
    // Lines 40 and 41, which the file never got
    const batch = (n, text) => JSON.stringify({ type: `build:log:${id}`, data: { taskID: 0, id: n, data: `[ 1s] ${text}\n` } });
    socket.send(batch(40, "n=40."));
    await expect(page.getByTestId("log-search-count")).toHaveText(/ of 2$/);
    socket.send(batch(41, "n=41."));
    await expect(page.getByTestId("log-search-count")).toHaveText(/ of 3$/);
    expect(reads).toBe(2);
    await gate.release();
});

test("should number a long log's tail from where it starts", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: Talk\n    run: echo hi\n");
    const id = await wake.runJob(name);
    await wake.waitForBuild(id);
    // As if the log were over 50 MB, and its tail started at line 391
    await page.route(
        (url) => url.pathname === `/api/build/${id}`,
        async (route) => {
            const response = await route.fetch();
            const json = await response.json();
            json.status_update.tasks[0].logSize = 60 * 1024 * 1024;
            await route.fulfill({ response, json });
        },
    );
    await page.route(
        (url) => url.pathname === `/api/build/${id}/log/0` && url.searchParams.has("tail"),
        (route) => route.fulfill({ body: "line 391\nline 392\nline 393", headers: { "Content-Type": "text/plain", "X-First-Line": "391" } }),
    );
    await page.goto(`/build/${id}#t0-L392`);
    const log = page.getByTestId("task-log");
    await expect(log.locator(".cm-lineNumbers .cm-gutterElement").filter({ hasText: /^391$/ })).toBeVisible();
    await expect(log.locator(".log-marked")).toHaveText("line 392");
});

// A running task may be writing its last line as the log is read; the
// live log sends it whole, so the viewer leaves the unfinished one out
test("should leave out an unfinished last line of a log", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: Talk\n    run: echo hi\n");
    const id = await wake.runJob(name);
    await wake.waitForBuild(id);
    await page.route(
        (url) => url.pathname === `/api/build/${id}/log/0`,
        (route) => route.fulfill({ body: "line 1\nline 2\nline 3 unfini", headers: { "Content-Type": "text/plain" } }),
    );
    await wake.openBuild(id);
    await openLog(page);
    const log = page.getByTestId("task-log");
    await expect(log.locator(".cm-line")).toHaveText(["line 1", "line 2"]);
});

test("should search all tasks and open a match", async ({ page, wake }) => {
    const name = await wake.createJob(`
tasks:
  - name: First
    run: echo "nee""dle in the first"
  - name: Second
    run: echo nothing here
  - name: Third
    run: echo "another NEE""DLE"
`);
    const id = await wake.runJob(name);
    await wake.openBuild(id);
    await page.getByTestId("search-all").fill("needle");
    await page.getByTestId("search-all").press("Enter");
    const results = page.getByTestId("search-all-results");
    await expect(results).toContainText("First1 match");
    await expect(results).toContainText("Secondno matches");
    await expect(page.getByTestId("search-all-hit")).toHaveCount(2);
    await page.getByTestId("search-all-hit").last().click();
    await expect(page).toHaveURL(new RegExp(`#t2-L2$`));
    await expect(page.getByTestId("task-log").locator(".log-marked")).toContainText("another NEEDLE");
});

// L2: a running task opened midway shows what it wrote, then each line
// that arrives, once
test("should show a running task's whole log, each line once", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
tasks:
  - name: Talk
    run: for i in $(seq 1 50); do echo "n""=$i."; done; ${gate.command}; for i in $(seq 51 55); do echo "n""=$i."; done; sleep 1
`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "running");
    await wake.livePage(`/build/${id}`);
    const log = page.getByTestId("task-log");
    await page.getByTestId("log-search-open").click();
    await page.getByTestId("log-search").fill("n=50.");
    await expect(page.getByTestId("log-search-count")).toHaveText("1 of 1");
    await gate.release();
    await page.getByTestId("log-search").fill("n=55.");
    await expect(page.getByTestId("log-search-count")).toHaveText("1 of 1");
    // Every line once; the current match is the one nearest the view
    await page.getByTestId("log-search").fill("n=");
    await expect(page.getByTestId("log-search-count")).toHaveText(/ of 55$/);
    await expect(log).toBeVisible();
});

// Two batches of live lines in one frame: the first waits for the next
// animation frame, and the second isn't a gap after it
test("should take two batches of live lines at once without reading the log again", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`
tasks:
  - name: Talk
    run: echo "n""=1."; ${gate.command}; for i in $(seq 2 4); do echo "n""=$i."; sleep 0.3; done; sleep 1
`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "running");
    let reads = 0;
    page.on("request", (request) => {
        if (new URL(request.url()).pathname.startsWith(`/api/build/${id}/log/`)) {
            reads++;
        }
    });
    let subscribed = false;
    const held = [];
    await page.routeWebSocket(/\/ws$/, (ws) => {
        const server = ws.connectToServer();
        ws.onMessage((message) => {
            subscribed ||= message.includes(`"build:log:${id}"`);
            server.send(message);
        });
        server.onMessage((message) => {
            if (held.length < 2 && message.includes(`"type":"build:log:${id}"`)) {
                held.push(message);
                if (held.length === 2) {
                    ws.send(held.join("\n"));
                }
                return;
            }
            ws.send(message);
        });
    });
    await page.goto(`/build/${id}`);
    await page.getByTestId("log-search-open").click();
    await page.getByTestId("log-search").fill("n=");
    await expect(page.getByTestId("log-search-count")).toHaveText("1 of 1");
    // Lines sent before the page subscribed would be a real gap
    await expect.poll(() => subscribed).toBe(true);
    await gate.release();
    await expect.poll(() => held.length).toBe(2);
    await expect(page.getByTestId("log-search-count")).toHaveText(/ of 4$/);
    expect(reads).toBe(1);
});

// The server keeps subscriptions per connection: after a reconnect the
// build page subscribes to its log and updates again
test("should subscribe to a build's log again after a reconnect", async ({ page, wake }) => {
    const gate = await wake.gate();
    const name = await wake.createJob(`tasks:\n  - name: Talk\n    run: ${gate.command}\n`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "running");
    const sockets = [];
    await page.routeWebSocket(/\/ws$/, (ws) => {
        const server = ws.connectToServer();
        const entry = { ws, logs: false };
        ws.onMessage((message) => {
            entry.logs ||= message.includes(`"build:log:${id}"`);
            server.send(message);
        });
        server.onMessage((message) => ws.send(message));
        sockets.push(entry);
    });
    await page.goto(`/build/${id}`);
    await expect.poll(() => sockets[0]?.logs).toBe(true);
    sockets[0].ws.close();
    await expect.poll(() => sockets.length, { timeout: 15000 }).toBe(2);
    await expect.poll(() => sockets[1].logs, { timeout: 15000 }).toBe(true);
    await gate.release();
});

// Artifacts can write the app's localStorage: a theme it doesn't know
// leaves the page on a known one, and a coloured log still opens
test("should open a coloured log with a stored theme it doesn't know", async ({ page, wake }) => {
    const name = await wake.createJob("tasks:\n  - name: Talk\n    run: printf '\\033[38;5;214mwa''rn\\033[0m\\n'\n");
    const id = await wake.runJob(name);
    await wake.waitForBuild(id);
    await page.addInitScript(() => window.localStorage.setItem("theme", "bogus"));
    await wake.openBuild(id);
    await openLog(page);
    // Coloured: the palette threw, and the log showed without colours
    await expect(page.getByTestId("task-log").locator("span[style*='color']", { hasText: "warn" })).toHaveCount(1);
});

test("should open a failed log at its end without moving the page", async ({ page, wake }) => {
    const name = await wake.createJob(`
tasks:
  - name: Long
    run: for i in $(seq 1 200); do echo "line $i"; done; echo "the last li""ne"; exit 1
`);
    const id = await wake.runJob(name);
    await wake.waitForBuild(id, "failed");
    await page.setViewportSize({ width: 390, height: 700 });
    await page.goto(`/build/${id}`);
    const log = page.getByTestId("task-log").first();
    await expect(log.getByText("the last line")).toBeInViewport();
    // The title stays in sight: only the viewer scrolled
    expect(await page.evaluate(() => window.scrollY)).toBe(0);
    await expect(page.locator("h1.title")).toBeInViewport();
});
