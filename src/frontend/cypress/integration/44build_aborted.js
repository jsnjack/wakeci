describe("Build page - Aborted", function () {
    it("should abort build when a user asks from the feed page", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Sleepy head
    run: sleep 5
`;

        cy.request({
            url: "/api/job/" + jobName,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                fileContent: jobContent,
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
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("tr")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=build-status-label]").should("contain", "running");
                cy.get("[data-cy=abort-build-button]").click();
                cy.get("[data-cy=build-status-label]").should("contain", "aborted");
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // One label for the build and one label for the task
                cy.get("[data-cy=build-status-label]").should("have.length", 2);
                cy.get("[data-cy=build-status-label]").should("contain", "aborted");
                cy.get("[data-cy=reload]").eq(0).click();
                cy.get("body").should("contain", "Aborted by a user.");
            });
    });

    it("should abort build when a user asks from the build page and run on_aborted tasks", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Sleepy head
    run: sleep 5

on_aborted:
  - name: Aborted
    run: echo "BINGO"
`;

        cy.request({
            url: "/api/job/" + jobName,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                fileContent: jobContent,
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
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("tr")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=build-status-label]").should("contain", "running");
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                cy.get("[data-cy=abort-build-button]").click();
                // One label for the build and two labels for the task
                cy.get("[data-cy=build-status-label]").should("have.length", 3);
                cy.get("[data-cy=build-status-label]").should("contain", "aborted");
                cy.get("[data-cy=reload]").eq(0).click();
                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("body").should("contain", "Aborted by a user.");
                cy.get("body").should("contain", "BINGO");
            });
    });

    it("should abort build automatically when timeout is reached and run on_aborted tasks", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Sleepy head
    run: sleep 5

on_aborted:
  - name: Aborted
    run: echo "BONGO"

timeout: 1s
`;

        cy.request({
            url: "/api/job/" + jobName,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                fileContent: jobContent,
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
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("tr")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=build-status-label]").should("contain", "timed out");
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // One label for the build and two labels for the task
                cy.get("[data-cy=build-status-label]").should("have.length", 3);
                cy.get("[data-cy=build-status-label]").should("contain", "timed out");
                cy.get("[data-cy=reload]").eq(0).click();
                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("body").should("contain", "Timed out.");
                cy.get("body").should("contain", "BONGO");
            });
    });
});
