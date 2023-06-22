describe("Build page - Include", function () {
    it("should include tasks from the template file and use their Env", function () {
        // Create a file with tasks to include
        const filePath = "/tmp/tasks.inc";
        const includeContent = `
- name: Included task 1
  run: echo "task 1"

- name: Included task 2
  run: printenv HELLO_VAR
  env:
    HELLO_VAR: JOE
`;
        cy.writeFile(filePath, includeContent);
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
desc: Include test
tasks:
  - name: Check parameters
    run: uname -a

  - include: ${filePath}
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
        // Reset include file content
        cy.writeFile(filePath, "");
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // Verify number of tasks
                cy.get("[data-cy=reload]").should("have.length", 3);
                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("body").should("contain", "task 1");
                cy.get("[data-cy=reload]").eq(2).click();
                cy.get("body").should("contain", "JOE");
            });
    });

    it("should include tasks from the template file and override their env", function () {
        // Create a file with tasks to include
        const filePath = "/tmp/tasks.inc";
        const includeContent = `
- name: Included task 1
  run: echo "task 1"

- name: Included task 2
  run: printenv HELLO_VAR
  env:
    HELLO_VAR: JOE
`;
        cy.writeFile(filePath, includeContent);
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
desc: Include test
tasks:
  - name: Check parameters
    run: uname -a

  - include: ${filePath}
    env:
      HELLO_VAR: PATRICK
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
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // Verify number of tasks
                cy.get("[data-cy=reload]").should("have.length", 3);
                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("body").should("contain", "task 1");
                cy.get("[data-cy=reload]").eq(2).click();
                cy.get("body").should("contain", "PATRICK");
            });
    });

    it("should include tasks from the template file and preserve their env", function () {
        // Create a file with tasks to include
        const filePath = "/tmp/tasks.inc";
        const includeContent = `
- name: Included task 1
  run: echo "task 1"

- name: Included task 2
  run: printenv HELLO_VAR
  env:
    HELLO_VAR: JOE
`;
        cy.writeFile(filePath, includeContent);
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
desc: Include test
tasks:
  - name: Check parameters
    run: uname -a

  - include: ${filePath}
    env:
      TEST: 1
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
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // Verify number of tasks
                cy.get("[data-cy=reload]").should("have.length", 3);
                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("body").should("contain", "task 1");
                cy.get("[data-cy=reload]").eq(2).click();
                cy.get("body").should("contain", "JOE");
            });
    });

    it("should include tasks from the template file and use original when", function () {
        // Create a file with tasks to include
        const filePath = "/tmp/tasks.inc";
        const includeContent = `
- name: Included task 1
  run: echo "task 1"
  when: $NAME == joe
`;
        cy.writeFile(filePath, includeContent);
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
desc: Include test
params:
  - NAME: joe
tasks:
  - include: ${filePath}
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
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // Verify number of tasks
                cy.get("[data-cy=reload]").should("have.length", 1);
                cy.get("[data-cy=reload]").click();
                cy.get("body").should("contain", "task 1");
            });
    });

    it("should include tasks from the template file and override original when", function () {
        // Create a file with tasks to include
        const filePath = "/tmp/tasks.inc";
        const includeContent = `
- name: Included task 1
  run: echo "task 1"
  when: $NAME == joe
`;
        cy.writeFile(filePath, includeContent);
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
desc: Include test
params:
  - NAME: tim
tasks:
  - include: ${filePath}
    when: $NAME == tim
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
        cy.get("[data-cy-build]")
            .invoke("attr", "data-cy-build")
            .then((val) => {
                cy.get("[data-cy=open-build-button]").click();
                cy.url().should("include", "/build/" + val);
                // Verify number of tasks
                cy.get("[data-cy=reload]").should("have.length", 1);
                cy.get("[data-cy=reload]").click();
                cy.get("body").should("contain", "task 1");
            });
    });
});
