describe("Jobs page", function () {
    it("should handle API network errors without crashing", function () {
        cy.intercept("GET", "/api/jobs/", { forceNetworkError: true }).as("jobsNetworkError");
        cy.visit("/jobs");
        cy.login();
        cy.wait("@jobsNetworkError");

        cy.location("pathname").should("eq", "/jobs");
        cy.get(".notification-content").should("contain", "Network Error");
        cy.get("[data-cy=jobs-container]").should("contain", "No jobs found");
    });

    it("should create a new job", function () {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`[data-cy=${jobName}]`).should("be.visible");

        // Should fail to create a job with the same name again
        cy.get("[data-cy=create-job]").click();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "Job with this name already exists");
    });

    it("should delete a job", function () {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`[data-cy=${jobName}]`).should("be.visible");

        cy.get(`[data-cy=${jobName}] [data-cy=delete-job-button]`).click();
        cy.get(`[data-cy=${jobName}] [data-cy=delete-job-confirm]`).click();
        cy.get(`[data-cy=${jobName}]`).should("not.exist");
    });

    it("should escape HTML in a job name shown in notifications", function () {
        // Regression test: notifications render as raw HTML (App.vue's
        // dangerouslySetInnerHtml, used so a scheduled-build notification can
        // include a clickable link), so any job name embedded in one used to
        // be interpreted as markup instead of text.
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobNamePrefix = "myjobxss" + new Date().getTime();
        const jobName = `${jobNamePrefix}<img src=x onerror="window.__xssTriggered = true">`;
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");

        cy.contains(".row", jobNamePrefix).find("[data-cy=run-job-button]").click();
        cy.contains(".row", jobNamePrefix).find("[data-cy=start-job-confirm]").click();
        cy.get(".notification-content").should("contain", "has been scheduled");

        cy.window().then((win) => {
            expect(win.__xssTriggered).to.be.undefined;
        });
    });

    it("should handle reserved URL characters in a job name", function () {
        const jobName = `myjob#?% ${new Date().getTime()}`;
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

        cy.visit("/jobs");
        cy.login();
        cy.contains(".row", jobName).find("[data-cy=run-job-button]").click();
        cy.contains(".row", jobName).find("[data-cy=start-job-confirm]").click();
        cy.get(".notification-content").should("contain", `${jobName} has been scheduled`);

        cy.contains(".row", jobName).find("[data-cy=edit-job-button]").click();
        cy.location("pathname").should("eq", `/job/${encodeURIComponent(jobName)}`);
        cy.get("[data-cy=editor] .cm-content").should("contain", "desc: New job");
        cy.get("[data-cy=save-button]").click();
        cy.get(".notification-content").should("contain", "Saved");

        cy.visit("/jobs");
        cy.contains(".row", jobName).find("[data-cy=delete-job-button]").click();
        cy.contains(".row", jobName).find("[data-cy=delete-job-confirm]").click();
        cy.contains(".row", jobName).should("not.exist");
    });

    it("should edit a job", function () {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`[data-cy=${jobName}]`).should("be.visible");

        cy.get(`[data-cy=${jobName}] [data-cy=edit-job-button]`).click();
        cy.url().should("include", "/job/" + jobName);
        for (let index = 0; index < 5; index++) {
            cy.get("[data-cy=editor] .cm-content").type("{selectall}").type("{selectall}").type("{backspace}");
        }
        cy.get("[data-cy=editor] .cm-content").type("desc: Empty job{enter}tasks: []");
        cy.get("[data-cy=save-button]").click();
        cy.get(".notification-content").should("contain", "Saved");
        cy.visit("/jobs");
        cy.get(`[data-cy=${jobName}]`).should("contain", "Empty job");
    });

    it("should start a job", function () {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`[data-cy=${jobName}]`).should("be.visible");

        cy.get(`[data-cy=${jobName}] [data-cy=run-job-button]`).click();
        cy.get(`[data-cy=${jobName}] [data-cy=start-job-confirm]`).click();
        cy.get(".notification-content").should("contain", `${jobName} has been scheduled`);
        cy.visit("/");
        cy.get("body").should("contain", jobName);
    });

    it("should edit a job and navigate", function () {
        cy.visit("/jobs");
        cy.login();
        cy.get("[data-cy=create-job]").click();
        const jobName = "myjob" + new Date().getTime();
        cy.get("input[name=new-job-name]").clear().type(jobName);
        cy.get("[data-cy=create-job-button]").click();
        cy.get(".notification-content").should("contain", "New job created");
        cy.get(`[data-cy=${jobName}]`).should("be.visible");

        cy.get(`[data-cy=${jobName}] [data-cy=edit-job-button]`).click();
        cy.url().should("include", "/job/" + jobName);
        cy.get("[data-cy=save-and-close-button]").click();
        cy.location("pathname").should("eq", "/jobs");
    });

    it("should filter jobs", function () {
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
                name: jobName,
            },
            form: true,
        });
        cy.login();
        cy.visit("/jobs");
        cy.get("[data-cy=filter]")
            .clear()
            .type(jobName + "bad");
        cy.get("[data-cy=jobs-container]").should("contain", "No jobs found");
        cy.get("[data-cy=filter]").click({force:true}).clear().type(jobName);
        cy.get("[data-cy=jobs-container]").should("have.length", 1);
    });
});
