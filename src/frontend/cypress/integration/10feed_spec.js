describe("Feed page", function() {
    it("should open Feed page", function() {
        cy.visit("/");
        cy.login();
        cy.get("a[href='/']").should("contain", "Feed");
        cy.get("a[href='/jobs']").should("contain", "Jobs");
        cy.get("a[href='/settings']").should("contain", "Settings");
    });

    it("should filter jobs", function() {
        cy.visit("/");
        const jobName = "myjob" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "name": jobName,
            },
            form: true,
        });
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get(".empty").should("contain", "Empty");
        cy.get("[data-cy=filter]").clear();
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("[data-cy=feed-tbody]").should("be.visible").should("have.length", 1);
        cy.get("[data-cy=filtered-updates]").should("not.be.visible");
    });

    it("should hide updates when filter is active", function() {
        cy.visit("/");
        const jobName = "myjob" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "name": jobName,
            },
            form: true,
        });
        const filteredJobName = "myjob-filtered-" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "name": filteredJobName,
            },
            form: true,
        });
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });
        // 2 times to be sure!
        cy.request({
            url: `/api/job/${filteredJobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });
        cy.request({
            url: `/api/job/${filteredJobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });
        cy.get("[data-cy=feed-tbody]").should("be.visible").should("have.length", 1);
        cy.get("[data-cy=filtered-updates]").should("be.visible");
    });

    it("should filter jobs with params", function() {
        cy.visit("/");
        const jobName = "myjob" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "name": jobName,
            },
            form: true,
        });
        const jobContent = `
desc: Test env variables
params:
  - bereza: brest
tasks:
- name: Print env
run: env
`;

        cy.request({
            url: "/api/job/" + jobName,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "fileContent": jobContent,
            },
            form: true,
        });
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });

        cy.login();
        cy.get("[data-cy=filter]").clear().type("bereza");
        cy.get("[data-cy=feed-tbody]").should("be.visible").should("have.length", 1);
    });

    it("should toggle params", function() {
        // Create job
        const jobName = "myjob" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "name": jobName,
            },
            form: true,
        });

        const jobContent = `
desc: Test env variables
params:
  - pruzhany: pruzhany
  - minsk: minsk
tasks:
- name: Print env
run: env
`;

        cy.request({
            url: "/api/job/" + jobName,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "fileContent": jobContent,
            },
            form: true,
        });

        // Create build
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });

        cy.visit("/");
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("tr").invoke("attr", "data-cy-build").then((val) => {
            // Default value
            cy.get("[data-cy=params-text]").should("contain", "pruzhany");
            // Next value
            cy.get("[data-cy=params-index-button]").click();
            cy.get("[data-cy=params-text]").should("contain", "minsk");
            // Clear, back to the default one
            cy.get("[data-cy=params-index-button-clean]").click();
            cy.get("[data-cy=params-text]").should("contain", "pruzhany");
            // Default value again (when not enough params)
            cy.get("[data-cy=params-index-button]").click();
            cy.get("[data-cy=params-index-button]").click();
            cy.get("[data-cy=params-text]").should("contain", "pruzhany");
        });
    });

    it("should toggle duration", function() {
        // Create job
        const jobName = "myjob" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "name": jobName,
            },
            form: true,
        });

        const jobContent = `
desc: Test env variables
tasks:
- name: Print env
run: env
`;

        cy.request({
            url: "/api/job/" + jobName,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "fileContent": jobContent,
            },
            form: true,
        });

        // Create build
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });

        cy.visit("/");
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("tr").invoke("attr", "data-cy-build").then((val) => {
            cy.get("[data-cy=duration]").should("contain", "sec").
                click().should("contain", "just now").
                click().should("contain", ":").
                click().should("contain", "sec");
        });
    });

    it("should preserve duration after reload", function() {
        // Create job
        const jobName = "myjob" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "name": jobName,
            },
            form: true,
        });

        const jobContent = `
desc: Test env variables
tasks:
- name: Print env
run: env
`;

        cy.request({
            url: "/api/job/" + jobName,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                "fileContent": jobContent,
            },
            form: true,
        });

        // Create build
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });

        cy.visit("/");
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("[data-cy=duration]").should("contain", "sec").click().should("contain", "just now");
        cy.visit("/");
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("[data-cy=duration]").should("contain", "just now").click().click().should("contain", "sec");
    });
})
;
