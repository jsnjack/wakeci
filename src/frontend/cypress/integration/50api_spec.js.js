describe("API", function () {
    it("should use internal API", function () {
        // Create job
        const jobName = "myjob" + new Date().getTime();
        const jobName2 = "@" + jobName;

        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                name: jobName,
            },
            form: true,
        });

        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                name: jobName2,
            },
            form: true,
        });

        const jobContent = `
desc: Test internal API
tasks:
  - name: Call child job
    run: curl -X POST "http://127.0.0.1:8081/internal/api/job/${jobName2}/run"
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
        cy.get("[data-cy=feed-container]").should("be.visible").should("have.length", 1);
        cy.get("[data-cy=filter]").clear().type(jobName2);
        cy.get("[data-cy=feed-container]").should("be.visible").should("have.length", 1);
    });
});
