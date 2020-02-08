describe("Creating a new job", function() {
    it("should create a new job", function() {
        cy.visit("http://localhost:8080/");
        cy.login();
        cy.get("a[href='/jobs']").should("contain", "Jobs").click();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`tr[data-cy=${jobName}]`).should("be.visible");
        // Should fail to create a job with the same name again
        cy.get("[data-cy=create-job]").click();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "Job with this name already exists");
    });
})
;
