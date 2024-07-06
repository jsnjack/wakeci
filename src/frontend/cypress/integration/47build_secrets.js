describe("Build page - secrets", function () {
    it("should use secrets and not expose them", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: New job
tasks:
  - name: Example of using secrets
    run: echo "My secret is {{ secrets.TEST_KEY_1 }} and my API key is {{secrets.TEST_KEY_2 }} and from env $API_KEY. Unknown secret {{secrets.UNKNOWN_KEY }}"
    env:
      API_KEY: "{{     secrets.TEST_KEY_1 }}"
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
        cy.get("[data-cy=filter]").click({force:true}).clear().type(jobName);
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // Verify number of tasks
                cy.get("[data-cy=reload]").should("have.length", 1);
                cy.get("[data-cy=reload]").eq(0).click();
                cy.get("body").should("contain", "My secret is ***REDACTED***");
                cy.get("body").should("contain", "my API key is ***REDACTED***");
                cy.get("body").should("contain", "and from env ***REDACTED***");
                cy.get("body").should("contain", "Unknown secret <no value>");
            });
    });
});
