describe("Build page - Block", function () {
    it("should handle block statement", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Check parameters
    run: uname -a

  - name: Included task 2
    block:
      - run: printenv HELLO_VAR
      - run: echo "hello"
    env:
      HELLO_VAR: JOE
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
                cy.get("body").should("contain", "JOE");
            });
    });

    it("should handle nested blocks", function () {
        const jobName = "myjob" + new Date().getTime();
        const jobContent = `
desc: Env test
tasks:
  - name: Block 1
    block:
      - name: Task 1-1
        run: echo "1-1"

      - name: Block 2
        block:
          - name: Task 2-1
            run: echo "2-2"

          - name: Task 2-2
            run: printenv JOE
            env:
              JOE: BAD
    env:
      JOE: SUPER
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
                cy.get("[data-cy=reload]").eq(2).click();
                cy.get("body").should("contain", "SUPER");
            });
    });

    it("should combine well include and block", function () {
        // Create a file with tasks to include
        const filePath = "/tmp/tasks.inc";
        const includeContent = `
      - name: Task JOE
        run: printenv JOE
        env:
          JOE: BAD
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
    desc: Env test
    tasks:
      - name: Block 1
        block:
          - name: Task 1-1
            run: echo "1-1"

          - name: Block 2
            block:
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
                cy.get("[data-cy=reload]").should("have.length", 2);
                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("body").should("contain", "BAD");
            });
    });

    it("should combine nested when", function () {
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
    desc: Env test
    params:
      - CITY: PRUZHANY
      - COUNTRY: BELARUS
    tasks:
      - name: Block
        block:
          - name: Print kernel information 1
            run: uname -a
            when: $COUNTRY = BELARUS

          - name: Print kernel information 2
            run: uname -a
            when: $COUNTRY = PRUZHANY
        when: $CITY = PRUZHANY
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
                cy.get("[data-cy=reload]").eq(0).click();
                cy.get("[data-cy=task_section_0]").should("contain", "> Condition is true");

                cy.get("[data-cy=reload]").eq(1).click();
                cy.get("[data-cy=task_section_1]").should("contain", "> Condition is false");
            });
    });
});
