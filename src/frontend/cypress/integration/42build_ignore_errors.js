describe("Build page - Ignore errors", function() {
    it("should not ignore errors if task fails", function() {
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
desc: Ignore errors test
tasks:
  - name: Print env variable
    run: printenv NON_EXISTING_VARIABLE
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
            cy.get("[data-cy=open-build-button]").click();
            cy.url().should("include", "/build/" + val);
            // Verify number of tasks
            cy.get("[data-cy=reload]").click();
            cy.get("body").should("contain", "> Exit code: 1");
            cy.get("body").should("contain", "failed");
        });
    });

    it("should ignore errors if task fails", function() {
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
desc: Ignore errors test
tasks:
  - name: Print env variable
    run: printenv NON_EXISTING_VARIABLE
    ignore_errors: yes
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
            cy.get("[data-cy=open-build-button]").click();
            cy.url().should("include", "/build/" + val);
            // Verify number of tasks
            cy.get("[data-cy=reload]").click();
            cy.get("body").should("contain", "> Exit code: 1");
            cy.get("body").should("contain", "> Ignorring exit code");
            cy.get("body").should("not.contain", "failed");
        });
    });
});
