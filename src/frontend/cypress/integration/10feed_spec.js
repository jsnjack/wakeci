describe("Feed page", function() {
    it("should open Feed page", function() {
        cy.visit("/");
        cy.login();
        cy.get("a[href='/']").should("contain", "Feed");
        cy.get("a[href='/jobs']").should("contain", "Jobs");
        cy.get("a[href='/settings']").should("contain", "Settings");
    });

    it("should filter jobs", function() {
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
                "name": jobName,
            },
            form: true,
        });
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get(".empty").should("contain", "Empty");
        cy.get("[data-cy=filter]").clear();
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
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("tbody").should("have.length", 1);
    });
})
;
