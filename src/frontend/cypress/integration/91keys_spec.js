describe("API keys page", function () {
    it("should create, use and revoke a read-only key", function () {
        cy.visit("/keys");
        cy.login();
        cy.location("pathname").should("eq", "/keys");
        cy.get("[data-cy=whoami]").should("contain", "admin");

        const name = "readkey" + new Date().getTime();
        cy.get("[data-cy=create-key-open]").click();
        cy.location("pathname").should("eq", "/keys/new");
        // Create stays enabled and explains what's missing instead
        cy.get("[data-cy=create-key]").should("not.be.disabled").click();
        cy.contains("Give the key a name").should("be.visible");
        cy.get("#key-name").clear().type(name);
        // Only "read" is preselected, and admin can't be granted to keys
        cy.get("[data-cy=scope-read]").should("be.checked");
        cy.get("[data-cy=scope-run]").should("not.be.checked");
        cy.get("[data-cy=scope-admin]").should("not.exist");
        cy.get("[data-cy=jobs-all]").should("be.checked");
        cy.get("[data-cy=create-key]").click();

        cy.get("[data-cy=new-key-token]")
            .invoke("val")
            .then((token) => {
                expect(token).to.match(/^wcik_[0-9a-f]{80}$/);
                const headers = { Authorization: `Bearer ${token}` };

                // An explicit key wins over the browser's session cookie,
                // which cy.request also sends
                cy.request({ url: "/api/feed", headers }).its("status").should("eq", 200);
                cy.request({ method: "POST", url: "/api/job/nonexistent/run", headers, failOnStatusCode: false }).then(
                    (response) => {
                        expect(response.status).to.eq(403);
                        expect(response.body).to.contain("run");
                    }
                );
                // Keys can't mint more keys
                cy.request({ url: "/api/keys/", headers, failOnStatusCode: false }).its("status").should("eq", 403);

                cy.get("[data-cy=new-key-done]").click();
                cy.location("pathname").should("eq", "/keys");
                cy.get("[data-cy=new-key-token]").should("not.exist");
                cy.get(`[data-cy=key-${name}]`).should("contain", "read").and("not.contain", "run").and("contain", "All jobs");

                cy.get(`[data-cy=key-${name}] [data-cy=revoke-key]`).click();
                cy.get("[data-cy=revoke-key-confirm]").click();
                cy.get(`[data-cy=key-${name}]`).should("not.exist");
                cy.request({ url: "/api/feed", headers, failOnStatusCode: false }).its("status").should("eq", 403);
            });
    });

    it("should limit a key to the jobs it was given", function () {
        const stamp = new Date().getTime();
        const allowed = `keyjob${stamp}_a`;
        const later = `keyjob${stamp}_b`;
        const other = `otherjob${stamp}`;
        const createJob = (name) => cy.request({ method: "POST", url: "/api/jobs/create", form: true, body: { name } });

        cy.visit("/keys/new");
        cy.login();
        cy.location("pathname").should("eq", "/keys/new");
        createJob(allowed);
        createJob(other);
        // Pick up the new jobs
        cy.reload();

        const name = "jobkey" + stamp;
        cy.get("#key-name").type(name);
        cy.get("[data-cy=scope-run]").check({ force: true });
        cy.get("[data-cy=jobs-selected]").check({ force: true });
        cy.get("[data-cy=create-key]").click();
        cy.get("[data-cy=jobs-count]").should("contain", "Select a job or enter a pattern");

        // A pattern selects the matching jobs, and only those
        cy.get("[data-cy=job-patterns]").type(`keyjob${stamp}_*`);
        cy.get(`[data-cy=job-${allowed}] input`).should("be.checked").and("be.disabled");
        cy.get(`[data-cy=job-${allowed}]`).should("contain", `keyjob${stamp}_*`);
        cy.get(`[data-cy=job-${other}] input`).should("not.be.checked");
        // Searching only narrows the list, it doesn't change the selection
        cy.get("[data-cy=job-search]").type(other);
        cy.get(`[data-cy=job-${allowed}]`).should("not.exist");
        cy.get(`[data-cy=job-${other}] input`).should("not.be.checked");
        cy.get("[data-cy=job-search]").clear();
        cy.get(`[data-cy=job-${allowed}] input`).should("be.checked");
        cy.get("[data-cy=create-key]").click();

        cy.get("[data-cy=new-key-token]")
            .invoke("val")
            .then((token) => {
                const headers = { Authorization: `Bearer ${token}` };
                const status = (method, url) =>
                    cy.request({ method, url, headers, failOnStatusCode: false }).its("status");

                status("GET", `/api/job/${allowed}`).should("eq", 200);
                status("GET", `/api/job/${other}`).should("eq", 403);
                status("POST", `/api/job/${other}/run`).should("eq", 403);
                cy.request({ url: "/api/jobs/", headers }).then((response) => {
                    const names = response.body.map((job) => job.name);
                    expect(names).to.include(allowed);
                    expect(names).not.to.include(other);
                });
                // Jobs added later that match the pattern are covered too
                createJob(later);
                status("GET", `/api/job/${later}`).should("eq", 200);

                cy.get("[data-cy=new-key-done]").click();
                cy.get(`[data-cy=key-${name}] [data-cy=key-jobs]`).should("contain", `keyjob${stamp}_*`);
                cy.get(`[data-cy=key-${name}] [data-cy=revoke-key]`).click();
                cy.get("[data-cy=revoke-key-confirm]").click();
                cy.get(`[data-cy=key-${name}]`).should("not.exist");
            });

        [allowed, later, other].forEach((job) => cy.request({ method: "DELETE", url: `/api/job/${job}` }));
    });

    it("should not let a hidden pattern block creation", function () {
        const name = "patternkey" + new Date().getTime();
        cy.visit("/keys/new");
        cy.login();
        cy.get("#key-name").type(name);
        cy.get("[data-cy=jobs-selected]").check({ force: true });
        cy.get("[data-cy=job-patterns]").type("a/b");
        cy.contains("Patterns can't contain / or \\").should("be.visible");
        cy.get("[data-cy=jobs-all]").check({ force: true });
        cy.get("[data-cy=create-key]").click();
        cy.get("[data-cy=new-key-token]").should("exist");
        cy.get("[data-cy=new-key-done]").click();
        cy.get(`[data-cy=key-${name}]`).should("contain", "All jobs");
        cy.get(`[data-cy=key-${name}] [data-cy=revoke-key]`).click();
        cy.get("[data-cy=revoke-key-confirm]").click();
    });

    it("should create one key however often Create is clicked", function () {
        const name = "onekey" + new Date().getTime();
        cy.visit("/keys/new");
        cy.login();
        cy.get("#key-name").type(name);
        cy.get("[data-cy=create-key]").dblclick();
        cy.get("[data-cy=new-key-token]").should("exist");
        cy.get("[data-cy=new-key-done]").click();
        cy.get(`[data-cy=key-${name}]`).should("have.length", 1);
        cy.get(`[data-cy=key-${name}] [data-cy=revoke-key]`).click();
        cy.get("[data-cy=revoke-key-confirm]").click();
    });

    it("should copy the key without the Clipboard API, as over plain HTTP", function () {
        const name = "copykey" + new Date().getTime();
        cy.visit("/keys/new");
        cy.login();
        cy.get("#key-name").type(name);
        cy.get("[data-cy=create-key]").click();
        cy.get("[data-cy=new-key-token]").should("exist");
        cy.window().then((win) => {
            Object.defineProperty(win.navigator, "clipboard", { value: undefined, configurable: true });
            cy.stub(win.document, "execCommand").withArgs("copy").returns(true);
        });
        cy.get("button[data-cy=copy-key][aria-label='Copy API key']").focus().type("{enter}");
        cy.get("[data-cy=copy-key]").should("contain", "check");
        cy.get("[data-cy=new-key-done]").click();
        cy.get(`[data-cy=key-${name}] [data-cy=revoke-key]`).click();
        cy.get("[data-cy=revoke-key-confirm]").click();
    });

    it("should hide Google sign-in when it isn't configured", function () {
        cy.visit("/login");
        cy.get("input#password").should("be.visible");
        cy.get("[data-cy=google-login]").should("not.exist");
    });
});


describe("Write key access", function () {
    it("requires access to all jobs when write is selected", function () {
        cy.visit("/keys/new");
        cy.login();
        cy.get("[data-cy=jobs-selected]").check({ force: true });
        cy.get("[data-cy=scope-write]").check({ force: true });
        cy.get("[data-cy=jobs-all]").should("be.checked");
        cy.get("[data-cy=jobs-selected]").should("be.disabled");
        cy.contains("Write access can run commands on the server").should("be.visible");
        cy.get("[data-cy=scope-write]").uncheck({ force: true });
        cy.get("[data-cy=jobs-selected]").should("not.be.disabled");
    });
});
