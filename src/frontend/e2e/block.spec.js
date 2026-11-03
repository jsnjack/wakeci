import { test, expect } from "./fixtures.js";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";

test("should handle block statement", async ({ page, wake }) => {
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
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(3);
    await page.locator("[data-cy=reload]").nth(1).click();
    await expect(page.locator("body")).toContainText("JOE");
});

test("should handle nested blocks", async ({ page, wake }) => {
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
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(3);
    await page.locator("[data-cy=reload]").nth(2).click();
    await expect(page.locator("body")).toContainText("SUPER");
});

test("should combine well include and block", async ({ page, wake }) => {
    const filePath = join(wake.dir, "tasks.inc");
    const includeContent = `
      - name: Task JOE
        run: printenv JOE
        env:
          JOE: BAD
    `;
    await writeFile(filePath, includeContent);
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
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(2);
    await page.locator("[data-cy=reload]").nth(1).click();
    await expect(page.locator("body")).toContainText("BAD");
});

test("should combine nested when", async ({ page, wake }) => {
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
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=task-title]").nth(0).click();
    await expect(page.locator("body")).toContainText("> Condition is true");
    await page.locator("[data-cy=task-title]").nth(0).click();
    await page.locator("[data-cy=task-title]").nth(1).click();
    await expect(page.locator("body")).toContainText("> Condition is false");
});

test("should combine nested if", async ({ page, wake }) => {
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
            if: test $COUNTRY == BELARUS

          - name: Print kernel information 2
            run: uname -a
            if: test $COUNTRY == PRUZHANY
        if: test $CITY == PRUZHANY
    `;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=task-title]").nth(0).click();
    await expect(page.locator("body")).toContainText("> Condition is true");
    await page.locator("[data-cy=task-title]").nth(0).click();
    await page.locator("[data-cy=task-title]").nth(1).click();
    await expect(page.locator("body")).toContainText("> Condition is false");
});

test("should combine both if and when", async ({ page, wake }) => {
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
        if: test $CITY == PRUZHANY
    `;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=task-title]").nth(0).click();
    await expect(page.locator("body")).toContainText("> Condition is true");
    await page.locator("[data-cy=task-title]").nth(0).click();
    await page.locator("[data-cy=task-title]").nth(1).click();
    await expect(page.locator("body")).toContainText("> Condition is false");
});
