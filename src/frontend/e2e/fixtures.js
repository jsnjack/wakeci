import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdir, mkdtemp, open, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { randomUUID } from "node:crypto";

async function unusedPort() {
    const server = createServer();
    server.listen(0, "127.0.0.1");
    await once(server, "listening");
    const { port } = server.address();
    await new Promise((done, reject) => server.close((error) => (error ? reject(error) : done())));
    return port;
}

async function stopServer(child, exited) {
    if (child.exitCode !== null || child.signalCode !== null) return;
    process.kill(-child.pid, "SIGTERM");
    const stopped = await Promise.race([exited.then(() => true), delay(2000, undefined, { ref: false }).then(() => false)]);
    if (!stopped) {
        process.kill(-child.pid, "SIGKILL");
        await exited;
    }
}

export const test = base.extend({
    authenticated: [true, { option: true }],

    backend: [
        async ({ playwright }, use) => {
            const root = await mkdtemp(join(tmpdir(), "wakeci-e2e-"));
            let child;
            let exited;
            let api;
            const log = await open(join(root, "backend.log"), "w");
            try {
                const port = await unusedPort();
                const url = `http://127.0.0.1:${port}`;
                await mkdir(join(root, "jobs"));
                await writeFile(join(root, "secrets.yaml"), "TEST_KEY_1: test1\nTEST_KEY_2: test2\n");
                await writeFile(
                    join(root, "Wakefile.yaml"),
                    [
                        `port: "${port}"`,
                        `workdir: ${JSON.stringify(join(root, "work"))}`,
                        `jobdir: ${JSON.stringify(join(root, "jobs"))}`,
                        `secretsfile: ${JSON.stringify(join(root, "secrets.yaml"))}`,
                    ].join("\n"),
                );
                child = spawn(resolve(import.meta.dirname, "../../../bin/wakeci"), ["--config", join(root, "Wakefile.yaml"), "--debug"], {
                    detached: true,
                    stdio: ["ignore", log.fd, log.fd],
                });
                exited = once(child, "exit");
                api = await playwright.request.newContext({ baseURL: url });
                await expect
                    .poll(
                        async () => {
                            if (child.exitCode !== null || child.signalCode !== null) {
                                throw new Error(`Backend exited: ${await readFile(join(root, "backend.log"), "utf8")}`);
                            }
                            try {
                                return (await api.get("/auth/methods", { timeout: 500 })).ok();
                            } catch (error) {
                                if (!error.message.includes("ECONNREFUSED")) throw error;
                                return false;
                            }
                        },
                        { timeout: 10000, intervals: [50, 100, 200] },
                    )
                    .toBe(true);
                const backend = { root, url, api };
                // A password change ends the other password sessions, this one too
                backend.logIn = async () => {
                    await expect(await api.post("/auth/login", { form: { password: "admin" } })).toBeOK();
                    backend.storageState = await api.storageState();
                };
                await backend.logIn();
                await use(backend);
            } finally {
                if (child?.pid) await stopServer(child, exited);
                if (api) await api.dispose();
                await log.close();
                await rm(root, { recursive: true, force: true });
            }
        },
        { scope: "worker" },
    ],

    baseURL: async ({ backend }, use) => use(backend.url),
    // The settings page checks GitHub for a newer release; tests answer it
    // themselves. Routes a test adds take precedence
    page: async ({ page }, use) => {
        await page.route("**/api/update/", (route) => route.fulfill({ json: { current: "1.0.0", latest: "1.0.0", available: false } }));
        await use(page);
    },
    storageState: async ({ backend, authenticated }, use) => {
        await use(authenticated ? backend.storageState : { cookies: [], origins: [] });
    },

    backendLog: [
        async ({ backend }, use, testInfo) => {
            await use();
            if (testInfo.status !== testInfo.expectedStatus) {
                await testInfo.attach("backend.log", { body: await readFile(join(backend.root, "backend.log")), contentType: "text/plain" });
            }
        },
        { auto: true },
    ],

    wake: async ({ backend, page }, use) => {
        const jobs = [];
        const builds = [];
        const dir = await mkdtemp(join(backend.root, "test-"));
        const wake = {
            url: backend.url,
            dir,
            trackJob(name = `job-${randomUUID()}`) {
                jobs.push(name);
                return name;
            },
            async createJob(content, name = `job-${randomUUID()}`) {
                jobs.push(name);
                const response =
                    content === undefined
                        ? await backend.api.post("/api/jobs/create", { form: { name } })
                        : await backend.api.post(`/api/job/${encodeURIComponent(name)}`, { form: { fileContent: content } });
                await expect(response).toBeOK();
                // Saving returns before the file watcher registers the job.
                await expect
                    .poll(
                        async () => {
                            const registered = await backend.api.get("/api/jobs/");
                            await expect(registered).toBeOK();
                            return ((await registered.json()) || []).some((job) => job.name === name);
                        },
                        { intervals: [10, 25, 50] },
                    )
                    .toBe(true);
                return name;
            },
            async runJob(name) {
                const response = await backend.api.post(`/api/job/${encodeURIComponent(name)}/run`);
                await expect(response).toBeOK();
                const id = await response.json();
                builds.push(id);
                return id;
            },
            async waitForBuild(id, status = "finished") {
                await expect
                    .poll(
                        async () => {
                            const response = await backend.api.get(`/api/build/${id}`);
                            await expect(response).toBeOK();
                            return (await response.json()).status_update.status;
                        },
                        { intervals: [50, 100, 200] },
                    )
                    .toBe(status);
            },
            async openBuild(id, status = "finished") {
                await wake.waitForBuild(id, status);
                await page.goto(`/build/${id}`);
                await expect(page.getByTestId("reload").first()).toBeVisible();
            },
            async livePage(path) {
                const subscribed = page.waitForEvent("websocket").then((socket) =>
                    socket.waitForEvent("framesent", {
                        predicate: ({ payload }) => JSON.parse(payload).type === "in:subscribe",
                    }),
                );
                await page.goto(path);
                await subscribed;
            },
            async showFeed(name) {
                await wake.livePage("/");
                await Promise.all([
                    page.waitForResponse((response) => {
                        const url = new URL(response.url());
                        return url.pathname === "/api/feed" && url.searchParams.get("filter") === name;
                    }),
                    page.getByTestId("filter").fill(name),
                ]);
            },
            async gate() {
                const path = join(dir, `release-${randomUUID()}`);
                return {
                    command: `while [ ! -f '${path}' ]; do sleep 0.05; done`,
                    release: () => writeFile(path, ""),
                };
            },
        };
        try {
            await use(wake);
        } finally {
            for (const name of jobs) {
                const response = await backend.api.delete(`/api/job/${encodeURIComponent(name)}`);
                expect([200, 404]).toContain(response.status());
            }
            for (const id of builds) {
                const response = await backend.api.get(`/api/build/${id}`);
                await expect(response).toBeOK();
                if (["running", "pending"].includes((await response.json()).status_update.status)) {
                    const aborted = await backend.api.post(`/api/build/${id}/abort`);
                    expect([200, 404]).toContain(aborted.status());
                    await expect
                        .poll(async () => (await (await backend.api.get(`/api/build/${id}`)).json()).status_update.status, { intervals: [50, 100, 200] })
                        .not.toMatch(/^(running|pending)$/);
                }
            }
            await rm(dir, { recursive: true, force: true });
        }
    },
});

export async function login(page, password = "admin") {
    await page.locator("#password").fill(password);
    await Promise.all([
        page.waitForResponse((response) => response.url().endsWith("/auth/login") && response.status() === 204),
        page.getByRole("button", { name: "Log in", exact: true }).click(),
    ]);
    await expect(page.getByTestId("logout")).toBeVisible();
}

export { expect };
