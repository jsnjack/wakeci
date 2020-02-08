describe("Creating a new job", function() {
    it("should create a new job", function() {
        cy.visit("/jobs");
        cy.login();
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

    it("should delete a job", function() {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`tr[data-cy=${jobName}]`).should("be.visible");

        cy.get(`tr[data-cy=${jobName}] [data-cy=delete-job-button]`).click();
        cy.get(`tr[data-cy=${jobName}] [data-cy=delete-job-confirm]`).click();
        cy.get(`tr[data-cy=${jobName}]`).should("not.exist");
    });

    it("should edit a job", function() {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`tr[data-cy=${jobName}]`).should("be.visible");

        cy.get(`tr[data-cy=${jobName}] [data-cy=edit-job-button]`).click();
        cy.url().should("include", "/job/" + jobName);
        for (let index = 0; index < 5; index++) {
            cy.get("[data-cy=editor] .CodeMirror-code").type("{selectall}").type("{selectall}").type("{backspace}");
        }
        cy.get("[data-cy=editor] .CodeMirror-code").type("desc: Empty job");
        cy.get("[data-cy=save-button]").click();
        cy.get(".notification-content").should("contain", "Saved");
        cy.visit("/jobs");
        cy.get(`tr[data-cy=${jobName}]`).should("contain", "Empty job");
    });

    it("should start a job", function() {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`tr[data-cy=${jobName}]`).should("be.visible");

        cy.get(`tr[data-cy=${jobName}] [data-cy=start-job-button]`).click();
        cy.get(`tr[data-cy=${jobName}] [data-cy=start-job-confirm]`).click();
        cy.get(".notification-content").should("contain", `${jobName} has been scheduled`);
        cy.visit("/");
        cy.get("body").should("contain", jobName);
    });
})
;
