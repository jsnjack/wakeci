describe("Build page - concurrency", function () {
    it("should run both builds concurrently", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Sleepy head
    run: sleep 5

concurrency: 0
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
        cy.get("[data-cy-status]").should("have.length", 2);
        cy.get("[data-cy-status]").should((items) => {
            expect(items, "2 items").to.have.length(2);
            expect(items.eq(0), "first item").to.contain("running");
            expect(items.eq(1), "second item").to.contain("running");
        });
    });

    it("should run only one build concurrently", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Sleepy head
    run: sleep 5

concurrency: 1
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
        cy.get("[data-cy-status]").should("have.length", 2);
        cy.get("[data-cy-status]").should((items) => {
            expect(items, "2 items").to.have.length(2);
            expect(items.eq(0), "first item").to.contain("pending");
            expect(items.eq(1), "second item").to.contain("running");
        });
    });
});
