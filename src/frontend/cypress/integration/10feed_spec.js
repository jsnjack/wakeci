describe('Feed page', function () {
    it('should open Feed page', function () {
        cy.visit('/');
        cy.login();
        cy.get("a[href='/']").should('contain', 'Feed');
        cy.get("a[href='/jobs']").should('contain', 'Jobs');
        cy.get("a[href='/settings']").should('contain', 'Settings');
    });

    it('should filter jobs', function () {
        cy.visit('/');
        const jobName = 'myjob' + new Date().getTime();
        cy.request({
            url: '/api/jobs/create',
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {
                name: jobName,
            },
            form: true,
        });
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=empty-feed]').should('contain', 'Empty');
        cy.get('[data-cy=filter]').clear();
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {},
            form: true,
        });
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=feed-items]').should('be.visible').should('have.length', 1);
    });

    it('should hide updates when filter is active', function () {
        cy.visit('/');
        const jobName = 'myjob' + new Date().getTime();
        cy.request({
            url: '/api/jobs/create',
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {
                name: jobName,
            },
            form: true,
        });
        const filteredJobName = 'myjob-filtered-' + new Date().getTime();
        cy.request({
            url: '/api/jobs/create',
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {
                name: filteredJobName,
            },
            form: true,
        });
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {},
            form: true,
        });
        cy.wait(500);
        // 2 times to be sure!
        cy.request({
            url: `/api/job/${filteredJobName}/run`,
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {},
            form: true,
        });
        cy.request({
            url: `/api/job/${filteredJobName}/run`,
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {},
            form: true,
        });
        cy.get('[data-cy=feed-items]').should('be.visible').should('have.length', 1);
    });

    it('should filter jobs with params', function () {
        cy.visit('/');
        const jobName = 'myjob' + new Date().getTime();
        cy.request({
            url: '/api/jobs/create',
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
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
            url: '/api/job/' + jobName,
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {
                fileContent: jobContent,
            },
            form: true,
        });
        cy.request({
            url: `/api/job/${jobName}/run`,
            method: 'POST',
            auth: {
                user: '',
                pass: 'admin',
            },
            body: {},
            form: true,
        });

        cy.login();
        cy.get('[data-cy=filter]').clear().type('bereza');
        cy.get('[data-cy=feed-items]').should('be.visible').should('have.length', 1);
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
  run: sleep 5

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
        cy.get("[data-cy=build-status-label]").should("have.length", 2);
        cy.get("[data-cy=build-status-label]").should((items) => {
            expect(items, "2 items").to.have.length(2);
            expect(items.eq(0), "first item").to.contain("pending");
            expect(items.eq(1), "second item").to.contain("running");
        });
        cy.get("[data-cy=start-build-button]").click();
        cy.get("[data-cy=build-status-label]").should((items) => {
            expect(items, "2 items").to.have.length(2);
            expect(items.eq(0), "first item").to.contain("running");
            expect(items.eq(1), "second item").to.contain("running");
        });
    });
});
