describe("Feed page", function () {
    it("should open Feed page", function () {
        cy.visit("/");
        cy.login();
        cy.get("a[href='/']").should("contain", "Feed");
        cy.get("a[href='/jobs']").should("contain", "Jobs");
        cy.get("a[href='/settings']").should("contain", "Settings");
    });

    it("should filter builds", function () {
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
        cy.get("[data-cy=filter]").clear().type(jobName);
        cy.get("[data-cy=no-builds-found]").should("contain", "No builds found");
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
        cy.get("[data-cy=feed-container]").should("have.length", 1);
        cy.get("[data-cy=filtered-updates]").should("not.be.visible");
    });

    it("should hide updates when filter is active", function () {
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
        const filteredJobName = "myjob-filtered-" + new Date().getTime();
        cy.request({
            url: "/api/jobs/create",
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {
                name: filteredJobName,
            },
            form: true,
        });
        cy.login();
        cy.get("[data-cy=filter]").clear().type(jobName);
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
        cy.wait(500);
        // 2 times to be sure!
        cy.request({
            url: `/api/job/${filteredJobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });
        cy.request({
            url: `/api/job/${filteredJobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });
        cy.get("[data-cy=feed-container]").should("be.visible").should("have.length", 1);
        cy.get("[data-cy=filtered-updates]").should("be.visible");
    });

    it("should filter jobs with params", function () {
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
        const jobContent = `
desc: Test env variables
params:
  - bereza: brest
tasks:
- name: Print env
run: env
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

        cy.login();
        cy.get("[data-cy=filter]").clear().type("bereza");
        cy.get("[data-cy=feed-container]").should("be.visible").should("have.length", 1);
    });

    it("should toggle params", function () {
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
desc: Test env variables
params:
  - param1: ok0
  - param2: ok1
  - param3: ok2
  - param4: ok3
tasks:
- name: Print env
run: env
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
        cy.get("[data-cy=params-container]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                // Default value
                cy.get("[data-cy=params-value]").each((item, index, list) => {
                    expect(list).to.have.length(3);
                    switch (index) {
                        case 0:
                            expect(item.text()).to.equal("ok0content_copyparam1");
                            break;
                        case 1:
                            expect(item.text()).to.equal("ok1content_copyparam2");
                            break;
                        case 2:
                            expect(item.text()).to.equal("ok2content_copyparam3");
                            break;
                    }
                });
                // Expand params
                cy.get("[data-cy=expand-more-params-button]").click();
                cy.get("[data-cy=params-value]").each((_1, _2, list) => {
                    expect(list).to.have.length(4);
                });

                // Collapse params
                cy.get("[data-cy=expand-less-params-button]").click();
                cy.get("[data-cy=params-value]").each((_1, _2, list) => {
                    expect(list).to.have.length(3);
                });
            });
    });

    it("should preserve filter after reload", function () {
        cy.visit("/");
        const jobName = "myjob " + new Date().getTime();
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
            url: `/api/job/${jobName}/run`,
            method: "POST",
            auth: {
                user: "",
                pass: "admin",
            },
            body: {},
            form: true,
        });
        cy.login();
        cy.get("[data-cy=filter]")
            .clear()
            .type('"' + jobName + '"');
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                cy.go("back");
                cy.get("[data-cy=open-build-button]").should("have.length", 1);
                cy.get("[data-cy=filter]").should("have.value", '"' + jobName + '"');
            });
    });

    it("should start job immediately", function () {
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
        const jobContent = `
desc: Test env variables
tasks:
- name: Print env
  run: sleep 10

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

        // Queue 2 jobs
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
        cy.get("[data-cy=start-build-button]:not([disabled])").click();
        cy.get("[data-cy-status]").should((items) => {
            expect(items, "2 items").to.have.length(2);
            expect(items.eq(0), "first item").to.contain("running");
            expect(items.eq(1), "second item").to.contain("running");
        });
    });
});
