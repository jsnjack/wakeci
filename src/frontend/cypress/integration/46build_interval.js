describe("Build page - interval", function () {
    it("should schedule a build with interval", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Linux info
    run: uname -a

interval: "@every 2s"

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
        cy.visit("/");
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("[data-cy-status]").should("have.length.at.least", 2);
        cy.get("[data-cy-status]").should((items) => {
            expect(items, "2 items").to.have.length(2);
            expect(items.eq(0), "first item").to.contain("finished");
            expect(items.eq(1), "second item").to.contain("finished");
        });
        cy.request({
            url: "/api/job/" + jobName,
            method: "DELETE",
            auth: {
                user: "",
                pass: "admin",
            },
        });
    });
});
