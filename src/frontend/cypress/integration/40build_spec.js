describe('Build page', function () {
    it('should show build page', function () {
        // Create job
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
        // Create build
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
        cy.visit('/');
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=open-build-button]').should('have.length', 1);
        cy.get('[data-cy=feed-item]')
            .invoke('attr', 'data-cy-build')
            .then((val) => {
                cy.get('[data-cy=open-build-button]').click();
                cy.url().should('include', '/build/' + val);
                cy.get('[data-cy=reload]').click();
                cy.get('.notification-content').should('contain', 'Log file has been reloaded');
                cy.get('body').should('contain', 'uname -a');
            });
    });

    it('should inject wake env variables', function () {
        // Create job
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
  - pruzhany: 5
  - minsk: 4
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

        // Create build
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

        cy.visit('/');
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=open-build-button]').should('have.length', 1);
        cy.get('[data-cy=feed-item]')
            .invoke('attr', 'data-cy-build')
            .then((val) => {
                cy.get('[data-cy=open-build-button]').click();
                cy.url().should('include', '/build/' + val);
                cy.get('[data-cy=reload]').click();
                cy.get('.notification-content').should('contain', 'Log file has been reloaded');
                cy.get('body').should('contain', `WAKE_JOB_NAME=${jobName}`);
                cy.get('body').should('contain', 'WAKE_URL=http://localhost:8081/');
                cy.get('body').should('contain', 'WAKE_CONFIG_DIR=');
                cy.get('body').should('contain', 'WAKE_BUILD_WORKSPACE=');
                cy.get('body').should('contain', 'WAKE_BUILD_ID=');
                cy.get('body').should('contain', 'WAKE_JOB_PARAMS=minsk=4&pruzhany=5');
            });
    });

    it('should collect artifacts', function () {
        // Create job
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
  - pruzhany: 5
  - minsk: 4
tasks:
  - name: Create 1 file
    run: journalctl -n 100 > big

  - name: Create 2 file
    run: journalctl -n 50 > small

artifacts:
  - "*"
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

        // Create build
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

        cy.visit('/');
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=open-build-button]').should('have.length', 1);
        cy.get('[data-cy=feed-item]')
            .invoke('attr', 'data-cy-build')
            .then((val) => {
                cy.get('[data-cy=open-build-button]').click();

                cy.get('[data-cy=artifactsMenu]').click();
                // Assert total number of artifacts
                cy.get('[data-cy^=artifact-]').should('have.length', 2);

                // Make sure Open index.html button is not present
                cy.get('[data-cy=openIndexFile]').should('not.exist');
            });
    });

    it('should display Open index.html button', function () {
        // Create job
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
  - pruzhany: 5
  - minsk: 4
tasks:
  - name: Create 1 file
    run: mkdir -p bb && journalctl -n 100 > bb/index.html

  - name: Create 2 file
    run: mkdir -p aa/cc && journalctl -n 50 > aa/cc/index.html

artifacts:
  - "**"
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

        // Create build
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

        cy.visit('/', {
            onBeforeLoad(win) {
                cy.stub(win, 'open').as('winOpen');
            },
        });
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=open-build-button]').should('have.length', 1);
        cy.get('[data-cy=feed-item]')
            .invoke('attr', 'data-cy-build')
            .then((val) => {
                cy.get('[data-cy=open-build-button]').click();

                cy.get('[data-cy=artifactsMenu]').click();

                // Make sure Open index.html button is present
                cy.get('[data-cy=openIndexFile]').should('contain', 'Open index.html');
                cy.get('[data-cy=openIndexFile]').click();
                cy.window()
                    .its('open')
                    .then((winOpen) => {
                        expect(winOpen.getCall(0).args[0]).to.includes('bb/index.html');
                    });
            });
    });

    it('should skip task when condition is false', function () {
        // Create job
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
desc: Condition test
tasks:
  - name: Print env
    run: env
    when: 1 == 2
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

        // Create build
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

        cy.visit('/');
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=open-build-button]').should('have.length', 1);
        cy.get('[data-cy=feed-item]')
            .invoke('attr', 'data-cy-build')
            .then((val) => {
                cy.get('[data-cy=open-build-button]').click();
                cy.url().should('include', '/build/' + val);
                cy.get('[data-cy=reload]').click();
                cy.get('.notification-content').should('contain', 'Log file has been reloaded');
                cy.get("body").should("contain", "skipped");
                cy.get('body').should('contain', 'Condition is false');
                cy.get('body').should('not.contain', 'WAKE_BUILD_ID=');
            });
    });

    it('should run task when condition is true', function () {
        // Create job
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
desc: Condition test
params:
 - NAME: joe
tasks:
  - name: Print env
    run: env
    when: $NAME == joe
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

        // Create build
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

        cy.visit('/');
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=open-build-button]').should('have.length', 1);
        cy.get('[data-cy=feed-item]')
            .invoke('attr', 'data-cy-build')
            .then((val) => {
                cy.get('[data-cy=open-build-button]').click();
                cy.url().should('include', '/build/' + val);
                cy.get('[data-cy=reload]').click();
                cy.get('.notification-content').should('contain', 'Log file has been reloaded');
                cy.get('body').should('contain', 'Condition is true');
                cy.get('body').should('contain', 'WAKE_BUILD_ID=');
            });
    });

    it('should inject env variables', function () {
        // Create job
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
desc: Env test
tasks:
  - name: Print env
    run: env
    env:
      NAME: joe
      SCORE: 5
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

        // Create build
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

        cy.visit('/');
        cy.login();
        cy.get('[data-cy=filter]').clear().type(jobName);
        cy.get('[data-cy=open-build-button]').should('have.length', 1);
        cy.get('[data-cy=feed-item]')
            .invoke('attr', 'data-cy-build')
            .then((val) => {
                cy.get('[data-cy=open-build-button]').click();
                cy.url().should('include', '/build/' + val);
                cy.get('[data-cy=reload]').click();
                cy.get('body').should('contain', 'NAME=joe');
                cy.get('body').should('contain', 'SCORE=5');
                cy.get('body').should('contain', 'WAKE_BUILD_ID=');
            });
    });

    it("should use build.env files", function () {
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
desc: build.env test
params:
  - NAME: bill
tasks:
  - name: Print NAME
    run: echo "This is $NAME"

  - name: Print NAME
    run: echo "His best friend is $NAME"
    env:
      NAME: joe

  - name: Create build.env file
    run: echo "NAME=big $NAME" >> build.env

  - name: Print new NAME
    run: echo "People call him $NAME"
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
        cy.get("[data-cy=open-build-button]").should("have.length", 1);
        cy.get("tr")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);

                cy.get("[data-cy=reload]").should("have.length", 4);
                cy.get("[data-cy=reload]").eq(0).click();
                cy.get("body").should("contain", "This is bill");
                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("body").should("contain", "His best friend is joe");
                cy.get("[data-cy=reload]").eq(3).click();
                cy.get("body").should("contain", "People call him big bill");
            });
    });
});
