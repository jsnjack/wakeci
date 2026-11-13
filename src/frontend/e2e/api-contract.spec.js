import { randomUUID } from "node:crypto";
import { test, expect } from "./fixtures.js";
import { documentedFields, validate } from "./openapi.js";

// The REST API as documented at /docs/swagger.json, checked against the
// running server: every operation is called, and every status, content type
// and JSON body must be what the spec says. API key scopes, the parameters
// and the fields of the responses are checked too.

const formBody = (fields) => {
    const body = new URLSearchParams();
    for (const [name, value] of Object.entries(fields)) {
        for (const v of [value].flat()) {
            body.append(name, v);
        }
    }
    return body.toString();
};

const contractJob = (id) => `desc: contract ${id}
params:
  - TARGET: staging
artifacts:
  - artifacts/**
tasks:
  - name: Test
    run: |
      mkdir -p artifacts
      echo '{"failed": ["test_x"]}' > artifacts/report.json
      echo "Failed: test_x" >> "$WAKE_SUMMARY"
      echo "FAILED test_x"
      exit 1
`;

test("should answer every documented operation as the spec says", async ({ wake, backend, playwright }) => {
    test.setTimeout(180000);
    const spec = await (await backend.api.get("/docs/swagger.json")).json();
    const ops = {};
    for (const [path, methods] of Object.entries(spec.paths)) {
        for (const [method, op] of Object.entries(methods)) {
            ops[`${method.toUpperCase()} ${path}`] = op;
        }
    }
    const problems = [];
    const seen = new Set();
    // Operations that answered at least once with success
    const succeeded = new Set();

    // The test runner gives new request contexts the project's storageState,
    // which holds the worker's session; these must not have it
    const fresh = (extraHTTPHeaders = {}) =>
        playwright.request.newContext({ baseURL: backend.url, storageState: { cookies: [], origins: [] }, extraHTTPHeaders });
    const contexts = {
        admin: backend.api,
        none: await fresh(),
    };
    const newKey = async (scopes) => {
        const response = await backend.api.post("/api/keys/", {
            data: formBody({ name: `contract-${scopes.join("-")}-${randomUUID()}`, scope: scopes, allJobs: "true", expiresInDays: "1" }),
            headers: { "Content-Type": "application/x-www-form-urlencoded" },
        });
        expect(response.status(), await response.text()).toBe(200);
        const { key, token } = await response.json();
        return { key, context: await fresh({ Authorization: `Bearer ${token}` }) };
    };

    // call sends a request for the documented operation opKey and checks the
    // answer against the spec
    const call = async (who, opKey, target, { form, headers, body } = {}) => {
        const op = ops[opKey];
        if (!op) {
            problems.push(`${opKey} is called but isn't documented`);
            return null;
        }
        const method = opKey.split(" ")[0];
        const options = { method, maxRedirects: 0, headers: { ...headers } };
        if (form) {
            options.data = formBody(form);
            options.headers["Content-Type"] = "application/x-www-form-urlencoded";
        }
        if (body !== undefined) {
            options.data = body;
            options.headers["Content-Type"] = "application/yaml";
        }
        const response = await contexts[who].fetch(target, options);
        const status = response.status();
        const label = `${method} ${target} as ${who} → ${status}`;
        const documented = op.responses[String(status)];
        if (!documented) {
            problems.push(`${label}: ${opKey} doesn't document ${status} (it documents ${Object.keys(op.responses).join(", ")})`);
        }
        if (status >= 200 && status < 300) {
            succeeded.add(opKey);
            const produces = (op.produces || [])[0] || "";
            const contentType = response.headers()["content-type"] || "";
            if (produces === "application/json" && !contentType.startsWith("application/json")) {
                problems.push(`${label}: Content-Type ${contentType}, documented as JSON`);
            }
            if (produces !== "application/json" && contentType.startsWith("application/json")) {
                problems.push(`${label}: Content-Type ${contentType}, documented as ${produces || "nothing"}`);
            }
            const schema = documented?.schema;
            if (schema && produces === "application/json") {
                validate(spec, schema, await response.json(), label, problems, seen);
            } else if (schema?.type === "integer" && !/^\d+$/.test((await response.text()).trim())) {
                problems.push(`${label}: body ${JSON.stringify(await response.text())}, documented as an integer`);
            }
        }
        return response;
    };
    const expectStatus = (response, statuses, what) => {
        if (response && !statuses.includes(response.status())) {
            problems.push(`${what}: got ${response.status()}, want ${statuses.join(" or ")}`);
        }
    };

    // Fixtures: a job with a failed build that wrote a log, an artifact and
    // a summary, and jobs to delete
    const uid = randomUUID().slice(0, 8);
    const job = await wake.createJob(contractJob(uid), `contract-${uid}`);
    const build = await wake.runJob(job);
    await wake.waitForBuild(build, "failed");
    const deletable = async () => wake.createJob(contractJob(randomUUID()), `contract-delete-${randomUUID().slice(0, 8)}`);
    const settings = await (await backend.api.get("/api/settings")).json();

    // How each operation succeeds; the ones that remove something get
    // something of their own to remove
    const valid = {
        "GET /api/feed": async () => ({ target: "/api/feed?limit=5" }),
        "GET /api/jobs/": async () => ({ target: "/api/jobs/" }),
        "POST /api/jobs/create": async () => ({ target: "/api/jobs/create", form: { name: `contract-new-${randomUUID().slice(0, 8)}` } }),
        "GET /api/job/{name}": async () => ({ target: `/api/job/${job}` }),
        "POST /api/job/{name}": async () => ({ target: `/api/job/${job}`, form: { fileContent: contractJob(uid) } }),
        "DELETE /api/job/{name}": async () => ({ target: `/api/job/${await deletable()}` }),
        "POST /api/job/{name}/run": async () => ({ target: `/api/job/${job}/run`, form: { TARGET: "contract" } }),
        "POST /api/job/{name}/set_active": async () => ({ target: `/api/job/${job}/set_active`, form: { active: "true" } }),
        "GET /api/build/{id}": async () => ({ target: `/api/build/${build}` }),
        "GET /api/build/{id}/summary": async () => ({ target: `/api/build/${build}/summary?tail=2` }),
        "GET /api/build/{id}/log/{task}": async () => ({ target: `/api/build/${build}/log/0?tail=1` }),
        // An ended build can't be aborted, flushed or started: 404 is their
        // documented answer, and a running build would end the test early
        "POST /api/build/{id}/abort": async () => ({ target: `/api/build/${build}/abort`, ended: true }),
        "POST /api/build/{id}/flush": async () => ({ target: `/api/build/${build}/flush`, ended: true }),
        "POST /api/build/{id}/start": async () => ({ target: `/api/build/${build}/start`, ended: true }),
        "GET /api/settings": async () => ({ target: "/api/settings" }),
        "POST /api/settings": async () => ({
            target: "/api/settings",
            form: { concurrentBuilds: String(settings.concurrentBuilds), buildHistorySize: String(settings.buildHistorySize) },
        }),
        "GET /api/settings/export": async () => ({ target: "/api/settings/export" }),
        // The server's own settings, so importing them changes nothing
        "POST /api/settings/import": async () => ({ target: "/api/settings/import", body: await (await backend.api.get("/api/settings/export")).text() }),
        "GET /api/queue/": async () => ({ target: "/api/queue/" }),
        // 409: the server isn't stopping, and stopping would end the test
        "POST /api/queue/abort": async () => ({ target: "/api/queue/abort", refused: true }),
        "GET /api/update/": async () => ({ target: "/api/update/" }),
        // Never a real version: it would install it
        "POST /api/update/": async () => ({ target: "/api/update/", form: { version: "0.0.0-contract" }, refused: true }),
        "GET /api/keys/": async () => ({ target: "/api/keys/" }),
        "POST /api/keys/": async () => ({
            target: "/api/keys/",
            form: { name: `contract-${randomUUID().slice(0, 8)}`, scope: "read", allJobs: "true", expiresInDays: "1" },
        }),
        "DELETE /api/keys/": async () => ({ target: "/api/keys/?createdBy=nobody%40example.com" }),
        "DELETE /api/keys/{id}": async () => ({ target: `/api/keys/${(await newKey(["read"])).key.id}` }),
        "GET /api/groups/": async () => ({ target: "/api/groups/" }),
        "POST /api/groups/": async () => ({ target: "/api/groups/", form: { name: `Contract ${randomUUID().slice(0, 8)}`, scope: "read", allJobs: "true" } }),
        "POST /api/groups/{name}": async () => {
            const name = `Contract ${randomUUID().slice(0, 8)}`;
            await call("admin", "POST /api/groups/", "/api/groups/", { form: { name, scope: "read", allJobs: "true" } });
            return { target: `/api/groups/${encodeURIComponent(name)}`, form: { scope: ["read", "run"], allJobs: "true" } };
        },
        "DELETE /api/groups/{name}": async () => {
            const name = `Contract ${randomUUID().slice(0, 8)}`;
            await call("admin", "POST /api/groups/", "/api/groups/", { form: { name, scope: "read", allJobs: "true" } });
            return { target: `/api/groups/${encodeURIComponent(name)}` };
        },
        "POST /api/nogroup": async () => ({ target: "/api/nogroup", form: { scope: "read", allJobs: "true" } }),
        "DELETE /api/nogroup": async () => ({ target: "/api/nogroup" }),
        "GET /api/whoami": async () => ({ target: "/api/whoami" }),
        "GET /api/analytics/": async () => ({ target: "/api/analytics/?days=7" }),
        "GET /api/job/{name}/insights": async () => ({ target: `/api/job/${job}/insights` }),
        "GET /storage/build/{id}/{path}": async () => ({ target: `/storage/build/${build}/task_0.log` }),
    };
    for (const opKey of Object.keys(ops)) {
        if (!valid[opKey]) {
            problems.push(`${opKey} is documented but this test doesn't call it; add it to valid`);
        }
    }

    // 1. The admin password: what succeeds, and the documented errors
    for (const [opKey, make] of Object.entries(valid)) {
        if (!ops[opKey]) {
            problems.push(`${opKey} is in this test but isn't documented`);
            continue;
        }
        const { target, form, body, ended, refused } = await make();
        const response = await call("admin", opKey, target, { form, body });
        if (ended) {
            expectStatus(response, [404], `${opKey} on an ended build`);
        } else if (refused) {
            expectStatus(response, [409, 502], `${opKey} with a version that isn't the latest`);
        } else {
            expectStatus(response, [200], opKey);
        }
    }
    const errors = [
        ["GET /api/feed", "/api/feed?limit=0", 400],
        ["GET /api/feed", "/api/feed?status=done", 400],
        ["POST /api/jobs/create", "/api/jobs/create", 400, { name: "not/valid" }],
        ["POST /api/jobs/create", "/api/jobs/create", 400, { name: job }],
        ["GET /api/job/{name}", `/api/job/nosuch-${uid}`, 404],
        ["POST /api/job/{name}", `/api/job/${job}`, 400, { fileContent: "tasks: [" }],
        ["DELETE /api/job/{name}", `/api/job/nosuch-${uid}`, 404],
        ["POST /api/job/{name}/run", `/api/job/nosuch-${uid}/run`, 400],
        ["POST /api/job/{name}/set_active", `/api/job/${job}/set_active`, 400, { active: "maybe" }],
        ["POST /api/job/{name}/set_active", `/api/job/nosuch-${uid}/set_active`, 404, { active: "true" }],
        ["GET /api/build/{id}", "/api/build/999999", 404],
        ["GET /api/build/{id}", "/api/build/x", 400],
        ["GET /api/build/{id}", `/api/build/${build}?wait=301`, 400],
        ["GET /api/build/{id}/summary", "/api/build/999999/summary", 404],
        ["GET /api/build/{id}/summary", `/api/build/${build}/summary?tail=-1`, 400],
        ["GET /api/build/{id}/log/{task}", `/api/build/${build}/log/7`, 404],
        ["GET /api/build/{id}/log/{task}", `/api/build/${build}/log/x`, 400],
        ["POST /api/build/{id}/abort", "/api/build/x/abort", 400],
        ["POST /api/build/{id}/flush", "/api/build/x/flush", 400],
        ["POST /api/build/{id}/start", "/api/build/x/start", 400],
        ["POST /api/settings", "/api/settings", 400, { concurrentBuilds: "x", buildHistorySize: "200" }],
        ["POST /api/keys/", "/api/keys/", 400, { name: "", scope: "read", allJobs: "true", expiresInDays: "1" }],
        ["DELETE /api/keys/", "/api/keys/", 400],
        ["DELETE /api/keys/{id}", "/api/keys/000000000000", 404],
        ["POST /api/groups/", "/api/groups/", 400, { name: "No scope", allJobs: "true" }],
        ["POST /api/groups/{name}", `/api/groups/Nosuch${uid}`, 404, { scope: "read", allJobs: "true" }],
        ["DELETE /api/groups/{name}", `/api/groups/Nosuch${uid}`, 404],
        ["POST /api/nogroup", "/api/nogroup", 400, { scope: "write", job: job }],
        ["POST /api/settings/import", "/api/settings/import", 400, undefined, "concurrentBuilds: 0\n"],
        ["GET /storage/build/{id}/{path}", `/storage/build/${build}/nosuch.txt`, 404],
        ["GET /api/analytics/", "/api/analytics/?days=0", 400],
        ["GET /api/analytics/", "/api/analytics/?trigger=robot", 400],
        ["GET /api/job/{name}/insights", `/api/job/${job}/insights?builds=0`, 400],
    ];
    for (const [opKey, target, status, form, body] of errors) {
        expectStatus(await call("admin", opKey, target, { form, body }), [status], `${opKey} ${target}`);
    }

    // 2. Without credentials, everything is refused with 401
    for (const [opKey, make] of Object.entries(valid)) {
        if (!ops[opKey]) {
            continue;
        }
        const { target, form, body } = await make();
        expectStatus(await call("none", opKey, target, { form, body }), [401], `${opKey} without credentials`);
    }

    // 3. API keys: the scope a route names lets a key in, any other scope
    // doesn't; "not available with an API key" refuses every key
    const scopes = ["read", "run", "write"];
    const keys = {};
    for (const scope of scopes) {
        keys[scope] = await newKey([scope]);
        keys[`not-${scope}`] = await newKey(scopes.filter((s) => s !== scope));
    }
    for (const [name, { context }] of Object.entries(keys)) {
        contexts[name] = context;
    }
    for (const [opKey, make] of Object.entries(valid)) {
        const op = ops[opKey];
        if (!op) {
            continue;
        }
        const description = op.description || "";
        const scope = description.match(/API key scope: (read|run|write)/)?.[1];
        const { target, form, ended, refused } = await make();
        if (/not available with an API key/i.test(description)) {
            expectStatus(await call("read", opKey, target, { form }), [403], `${opKey} with an API key`);
        } else if (scope) {
            const allowed = ended ? [404] : refused ? [409, 502] : [200];
            expectStatus(await call(scope, opKey, target, { form }), allowed, `${opKey} with a ${scope} key`);
            const again = await make();
            expectStatus(await call(`not-${scope}`, opKey, again.target, { form: again.form }), [403], `${opKey} with a key without ${scope}`);
        } else if (/Any API key/.test(description)) {
            expectStatus(await call("read", opKey, target, { form }), [200], `${opKey} with an API key`);
        } else {
            problems.push(`${opKey} doesn't say whether API keys may use it`);
        }
    }

    // 4. The parameters do what the spec says
    const json = async (target, who = "admin") => (await contexts[who].get(target)).json();
    const ids = (builds) => builds.map((b) => b.id);
    if ((await json("/api/feed?limit=1")).length !== 1) {
        problems.push("feed limit=1 doesn't return one build");
    }
    if (!ids(await json(`/api/feed?job=${job}&status=failed`)).includes(build)) {
        problems.push("feed job= and status= don't find the failed build");
    }
    if (ids(await json(`/api/feed?after=${build}`)).includes(build) || !ids(await json(`/api/feed?before=${build + 1}&limit=1`)).includes(build)) {
        problems.push("feed after= and before= don't bound the IDs");
    }
    const etag = (await backend.api.get("/api/feed?limit=3")).headers()["etag"];
    expectStatus(await call("admin", "GET /api/feed", "/api/feed?limit=3", { headers: { "If-None-Match": etag } }), [304], "feed If-None-Match");
    const summary = await json(`/api/build/${build}/summary?tail=1`);
    if (summary.failed?.[0]?.tail !== "FAILED test_x" && !String(summary.failed?.[0]?.tail).endsWith("Exit code: 1")) {
        problems.push(`summary tail=1 gives ${JSON.stringify(summary.failed?.[0]?.tail)}`);
    }
    if (!summary.jobSummary.includes("Failed: test_x") || summary.artifacts[0]?.url !== `/storage/build/${build}/artifacts/artifacts/report.json`) {
        problems.push(`summary misses the job's summary or the artifact URL: ${JSON.stringify(summary)}`);
    }
    const grep = await (await backend.api.get(`/api/build/${build}/log/0?grep=failed`)).text();
    if (!/^\d+: .*FAILED test_x/m.test(grep)) {
        problems.push(`log grep=failed gives ${JSON.stringify(grep)}`);
    }
    const ranged = await backend.api.get(`/storage/build/${build}/task_0.log`, { headers: { Range: "bytes=-5" } });
    expectStatus(ranged, [206], "storage Range");
    const started = Date.now();
    await backend.api.get(`/api/build/${build}?wait=30`);
    if (Date.now() - started > 5000) {
        problems.push("build wait=30 waited for an ended build");
    }
    const runResponse = await backend.api.post(`/api/job/${job}/run`, {
        data: formBody({ TARGET: "contract" }),
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
    });
    const run = Number(await runResponse.text());
    const ran = await json(`/api/build/${run}`);
    if (!ran.status_update.params.some((p) => p.TARGET === "contract")) {
        problems.push(`run TARGET=contract isn't in the build's params: ${JSON.stringify(ran.status_update.params)}`);
    }
    await wake.waitForBuild(run, "failed");

    // Lists again, now that there are groups and the keys have been used
    await backend.api.post("/api/nogroup", {
        data: formBody({ scope: "read", allJobs: "true" }),
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
    });
    expectStatus(await call("admin", "GET /api/groups/", "/api/groups/"), [200], "GET /api/groups/ with groups");
    await backend.api.delete("/api/nogroup");
    expectStatus(await call("admin", "GET /api/keys/", "/api/keys/"), [200], "GET /api/keys/ with used keys");

    // Analytics again, now that keys started builds and the job ran with two
    // values of TARGET, once those builds ended
    await expect.poll(async () => (await json("/api/analytics/?days=7")).people.some((p) => p.keyName), { intervals: [100, 200, 500] }).toBe(true);
    expectStatus(await call("admin", "GET /api/analytics/", "/api/analytics/?days=7"), [200], "GET /api/analytics/ with builds started by keys");
    await expect.poll(async () => (await json(`/api/job/${job}/insights`)).params.length, { intervals: [100, 200, 500] }).toBe(1);
    expectStatus(await call("admin", "GET /api/job/{name}/insights", `/api/job/${job}/insights`), [200], "GET /api/job/{name}/insights with two TARGETs");

    // 5. Every documented field shows up in some answer, except those of
    // states this test doesn't reach; Go tests and other specs cover them
    const conditional = {
        "main.APIKeysData.people": "someone signed in with single sign-on, or a key they made",
        "main.APIKeysData.signedIn": "someone signed in with single sign-on",
        "main.WhoAmIData.groups": "a single sign-on session",
        "main.QueueStatusData.abortAt": "a stopping server",
        "main.QueueStatusData.restartVersion": "a server restarting after an update",
        "main.QueueStatusData.buildsAborted": "a stopping server that aborted its builds",
        "main.UpdateData.error": "a failed check for a release",
        "main.UpdateData.problem": "a server that can't update itself",
        "main.ServerSSOData.idpMetadata": "a server with single sign-on",
        "main.ServerSSOData.acsUrl": "a server with single sign-on",
        "main.ServerSSOData.entityId": "a server with single sign-on",
        "main.ServerSSOData.certificates": "a server with single sign-on",
        "main.IDPCertificate.subject": "a server with single sign-on",
        "main.IDPCertificate.notAfter": "a server with single sign-on",
    };
    const unseen = documentedFields(spec).filter((field) => !seen.has(field) && !conditional[field]);

    for (const context of Object.entries(contexts)) {
        if (context[0] !== "admin") {
            await context[1].dispose();
        }
    }
    expect(problems, problems.join("\n")).toEqual([]);
    expect(unseen, `documented fields no answer had:\n${unseen.join("\n")}`).toEqual([]);
    expect(
        [...Object.keys(ops)].filter(
            (op) =>
                !succeeded.has(op) &&
                ![
                    "POST /api/build/{id}/abort",
                    "POST /api/build/{id}/flush",
                    "POST /api/build/{id}/start",
                    "POST /api/update/",
                    "POST /api/queue/abort",
                ].includes(op),
        ),
    ).toEqual([]);
});
