describe("Notifications", function () {
    it("should request notification permission and subscribe", function () {
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
                name: jobName,
            },
            form: true,
        });

        const jobContent = `
desc: Test notification subscription
tasks:
  - name: Long task
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

        cy.visit("/", {
            onBeforeLoad(win) {
                cy.stub(win.Notification, "permission", "default")
                cy.stub(win.Notification, "requestPermission").resolves("granted").as("ask")
                cy.stub(win, "Notification").as("Notification")
            },
        });
        cy.login();

        // Navigate to build page
        cy.get("[data-cy=filter]").click({force:true}).clear().type(jobName);
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((buildId) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + buildId);

                // Find subscribe button - it should show notifications_none icon initially
                cy.get("[data-cy=subscribe-button] i").should("contain", "notifications_none");

                // Click to subscribe
                cy.get("[data-cy=subscribe-button]").click();

                // Verify requestPermission was called
                cy.get("@ask").should("have.been.calledOnce");

                // Should show notifications_active icon after subscribing
                cy.get("[data-cy=subscribe-button] i").should("contain", "notifications_active");

                // Click again to unsubscribe
                cy.get("[data-cy=subscribe-button]").click();

                // Should show notifications_none icon after unsubscribing
                cy.get("[data-cy=subscribe-button] i").should("contain", "notifications_none");
            });
    });

    it("should show alert if browser does not support notifications", function () {
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
                name: jobName,
            },
            form: true,
        });

        const jobContent = `
desc: Test notification subscription
tasks:
  - name: Long task
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

        cy.visit("/", {
            onBeforeLoad(win) {
                delete win.Notification
            },
        });

        cy.on("window:alert", cy.stub().as("alerted"));
        cy.login();

        // Navigate to build page
        cy.get("[data-cy=filter]").click({force:true}).clear().type(jobName);
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((buildId) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + buildId);

                // Click to subscribe
                cy.get("[data-cy=subscribe-button]").click();

                // Verify alert was called
                cy.get("@alerted")
                    .should("have.been.calledOnce")
                    .and("have.been.calledWith", "This browser does not support system notifications");
            });
    });

    it("should subscribe even if permission was already granted", function () {
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
                name: jobName,
            },
            form: true,
        });

        const jobContent = `
desc: Test notification subscription
tasks:
  - name: Long task
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

        cy.visit("/", {
            onBeforeLoad(win) {
                // Notification permission already granted
                cy.stub(win.Notification, "permission", "granted")
                cy.stub(win.Notification, "requestPermission").resolves("granted").as("ask")
                cy.stub(win, "Notification").as("Notification")
            },
        });
        cy.login();

        // Navigate to build page
        cy.get("[data-cy=filter]").click({force:true}).clear().type(jobName);
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((buildId) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + buildId);

                // Click to subscribe
                cy.get("[data-cy=subscribe-button]").click();

                // requestPermission is still called even though permission was already granted
                cy.get("@ask").should("have.been.calledOnce");

                // Should show notifications_active icon after subscribing
                cy.get("[data-cy=subscribe-button] i").should("contain", "notifications_active");
            });
    });

    it("should not subscribe if permission was denied", function () {
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
                name: jobName,
            },
            form: true,
        });

        const jobContent = `
desc: Test notification subscription
tasks:
  - name: Long task
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

        cy.visit("/", {
            onBeforeLoad(win) {
                cy.stub(win.Notification, "permission", "default")
                cy.stub(win.Notification, "requestPermission").resolves("denied").as("ask")
                cy.stub(win, "Notification").as("Notification")
            },
        });
        cy.login();

        // Navigate to build page
        cy.get("[data-cy=filter]").click({force:true}).clear().type(jobName);
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((buildId) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + buildId);

                // Click to subscribe
                cy.get("[data-cy=subscribe-button]").click();

                // requestPermission was called but user denied
                cy.get("@ask").should("have.been.calledOnce");

                // Should still show notifications_none icon
                cy.get("[data-cy=subscribe-button] i").should("contain", "notifications_none");
            });
    });
});
