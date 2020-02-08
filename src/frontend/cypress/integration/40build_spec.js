describe("Build page", function() {
    it("should show build page", function() {
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
            cy.get("[data-cy=reload]").click();
            cy.get(".notification-content").should("contain", "Log file has been reloaded");
            cy.get("body").should("contain", "uname -a");
        });
    });
})
;
