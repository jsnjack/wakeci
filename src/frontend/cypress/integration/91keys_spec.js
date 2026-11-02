const admin = { user: "", pass: "admin" };

function adminRequest(method, url, body) {
    return cy.request({ method, url, auth: admin, body, form: true });
}

function keyRequest(token, method, url, body) {
    return cy.request({
        method,
        url,
        headers: { Authorization: "Bearer " + token },
        body,
        form: true,
        failOnStatusCode: false,
    });
}

describe("API keys", function () {
    const suffix = new Date().getTime();
    const deploy = "keydeploy" + suffix;
    const other = "keyother" + suffix;

    before(function () {
        for (const name of [deploy, other]) {
            adminRequest("POST", "/api/jobs/create", { name });
            adminRequest("POST", "/api/job/" + name, {
                fileContent: `desc: ${name}\ntasks:\n  - name: Say hi\n    run: echo hi from ${name}\n`,
            });
        }
    });

    it("should create a key that only works on its jobs", function () {
        cy.visit("/keys");
        cy.login();
        cy.get("[data-cy=nav-keys]").should("be.visible");
        cy.get("[data-cy=create-key-open]").click();
        cy.location("pathname").should("eq", "/keys/new");

        cy.get("[data-cy=key-name]").type("deploy " + suffix);
        cy.get("[data-cy=scope-run]").check({ force: true });
        cy.get("[data-cy=jobs-selected]").check({ force: true });
        cy.get(`[data-cy=key-job-${deploy}] input`).check({ force: true });
        cy.get("[data-cy=key-jobs-count]").should("contain", "1 of");
        cy.get("[data-cy=create-key]").click();

        cy.get("[data-cy=new-key-token]")
            .invoke("val")
            .then((token) => {
                expect(token).to.match(/^wk_[0-9a-f]{12}_[A-Za-z0-9_-]{43}$/);
                cy.get("[data-cy=new-key-example]").should("contain", "/api/feed");

                keyRequest(token, "GET", "/api/jobs/").then((response) => {
                    expect(response.status).to.eq(200);
                    expect(response.body.map((job) => job.name)).to.deep.equal([deploy]);
                });
                keyRequest(token, "POST", `/api/job/${other}/run`).its("status").should("eq", 403);
                keyRequest(token, "GET", "/api/settings").its("status").should("eq", 403);
                keyRequest(token, "GET", "/api/keys/").its("status").should("eq", 403);
                keyRequest(token, "POST", `/api/job/${deploy}/run`, { X: "{{ secrets.A }}" }).its("status").should("eq", 400);

                keyRequest(token, "POST", `/api/job/${deploy}/run`).then((response) => {
                    expect(response.status).to.eq(200);
                    keyRequest(token, "GET", "/api/build/" + response.body)
                        .its("status")
                        .should("eq", 200);
                });
                adminRequest("POST", `/api/job/${other}/run`).then((response) => {
                    keyRequest(token, "GET", "/api/build/" + response.body)
                        .its("status")
                        .should("eq", 403);
                    keyRequest(token, "GET", `/storage/build/${response.body}/`).its("status").should("eq", 403);
                });
            });

        cy.get("[data-cy=new-key-done]").click();
        cy.location("pathname").should("eq", "/keys");
        cy.get(`[data-cy='key-deploy ${suffix}']`).should("contain", "read").and("contain", "run");
        cy.get(`[data-cy='key-deploy ${suffix}'] [data-cy=key-jobs]`).should("have.text", deploy);
        cy.get(`[data-cy='key-deploy ${suffix}']`).should("contain", "Used");
    });

    it("should select jobs by pattern", function () {
        cy.visit("/keys/new");
        cy.login();
        cy.get("[data-cy=jobs-selected]").check({ force: true });
        cy.get("[data-cy=key-patterns]").type("keydeploy*");
        cy.get(`[data-cy=key-job-${deploy}]`).should("contain", "keydeploy*");
        cy.get(`[data-cy=key-job-${deploy}] input`).should("be.checked").and("be.disabled");
        cy.get(`[data-cy=key-job-${other}] input`).should("not.be.checked").and("not.be.disabled");
    });

    it("should keep write keys on all jobs", function () {
        cy.visit("/keys/new");
        cy.login();
        cy.get("[data-cy=jobs-selected]").check({ force: true });
        cy.get("[data-cy=key-patterns]").should("be.visible");
        cy.get("[data-cy=scope-write]").check({ force: true });
        cy.get("[data-cy=jobs-all]").should("be.checked");
        cy.get("[data-cy=jobs-selected]").should("be.disabled");
        cy.get("[data-cy=key-patterns]").should("not.exist");

        // Unticking write gives the job choice back
        cy.get("[data-cy=scope-write]").uncheck({ force: true });
        cy.get("[data-cy=jobs-selected]").should("be.checked").and("not.be.disabled");
        cy.get("[data-cy=key-patterns]").should("be.visible");
    });

    it("should only check the job fields while they are shown", function () {
        cy.visit("/keys/new");
        cy.login();
        cy.get("[data-cy=key-name]").type("hidden " + suffix);
        cy.get("[data-cy=jobs-selected]").check({ force: true });
        cy.get("[data-cy=key-patterns]").type("bad/pattern");
        cy.get("[data-cy=create-key]").click();
        cy.get("body").should("contain", "Patterns can't contain");
        cy.get("[data-cy=new-key-token]").should("not.exist");

        cy.get("[data-cy=jobs-all]").check({ force: true });
        cy.get("[data-cy=create-key]").click();
        cy.get("[data-cy=new-key-token]").should("be.visible");
    });

    it("should revoke a key", function () {
        adminRequest("POST", "/api/keys/", {
            name: "revoke " + suffix,
            scope: "read",
            allJobs: "true",
            expiresInDays: "0",
        }).then((response) => {
            const token = response.body.token;
            keyRequest(token, "GET", "/api/jobs/").its("status").should("eq", 200);

            cy.visit("/keys");
            cy.login();
            cy.get(`[data-cy='key-revoke ${suffix}']`).should("contain", "Never expires");
            // Logged in with the admin password: every key shows who made it
            cy.get(`[data-cy='key-revoke ${suffix}'] [data-cy=key-created-by-text]`).should("have.text", "by admin password");
            cy.get(`[data-cy='key-revoke ${suffix}'] [data-cy=revoke-key]`).click();
            cy.get("[data-cy=revoke-key-confirm]").click();
            cy.get(".notification-content").should("contain", "Key revoked");
            cy.get(`[data-cy='key-revoke ${suffix}']`).should("not.exist");
            keyRequest(token, "GET", "/api/jobs/").its("status").should("eq", 401);
        });
    });

    it("should let the admin password revoke all keys of a person", function () {
        const key = (id, name, createdBy) => ({
            id,
            name,
            createdBy,
            scopes: ["read"],
            allJobs: true,
            jobs: [],
            patterns: [],
            createdAt: new Date().toISOString(),
        });
        cy.intercept("GET", "/api/keys/", {
            admin: true,
            keys: [
                key("000000000001", "alice-ci", "alice@example.com"),
                key("000000000002", "alice-bot", "alice@example.com"),
                key("000000000003", "bob-ci", "bob@example.com"),
                key("000000000004", "shared", ""),
            ],
        });
        cy.intercept("DELETE", "/api/keys/?createdBy=alice%40example.com", { keys: 2, sessions: 1 }).as("revokePerson");
        cy.visit("/keys");
        cy.login();
        cy.get("[data-cy=revoke-person]").should("not.exist");
        cy.get("[data-cy=key-created-by]").select("alice@example.com");
        cy.get("[data-cy=keys-list]").should("contain", "alice-ci").and("not.contain", "bob-ci");
        cy.get("[data-cy=revoke-person]").should("contain", "alice@example.com").click();
        cy.get("[data-cy=revoke-person-confirm]").click();
        cy.wait("@revokePerson");
        cy.get(".notification-content").should("contain", "Revoked 2 keys of alice@example.com and signed them out");

        // Keys made with the admin password can be listed, but are revoked one by one
        cy.get("[data-cy=key-created-by]").select("Admin password");
        cy.get("[data-cy=keys-list]").should("contain", "shared").and("not.contain", "alice-ci");
        cy.get("[data-cy=revoke-person]").should("not.exist");
    });

    it("should show an SSO user only their own keys", function () {
        cy.intercept("GET", "/api/keys/", {
            admin: false,
            keys: [
                {
                    id: "000000000001",
                    name: "my-ci",
                    createdBy: "alice@example.com",
                    scopes: ["run"],
                    allJobs: true,
                    jobs: [],
                    patterns: [],
                    createdAt: new Date().toISOString(),
                },
            ],
        });
        cy.visit("/keys");
        cy.login();
        cy.get("[data-cy=keys-list]").should("contain", "my-ci");
        cy.get("[data-cy=key-created-by]").should("not.exist");
        cy.get("[data-cy=key-created-by-text]").should("not.exist");
    });
});
