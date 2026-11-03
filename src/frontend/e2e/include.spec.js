import { test, expect } from "./fixtures.js";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";

test("should include tasks from the template file and use their Env", async ({ page, wake }) => {
    const filePath = join(wake.dir, "tasks.inc");
    const includeContent = `
- name: Included task 1
  run: echo "task 1"

- name: Included task 2
  run: printenv HELLO_VAR
  env:
    HELLO_VAR: JOE
`;
    await writeFile(filePath, includeContent);
    const jobContent = `
desc: Include test
tasks:
  - name: Check parameters
    run: uname -a

  - include: ${filePath}
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await writeFile(filePath, "");
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(3);
    await page.locator("[data-cy=reload]").nth(1).click();
    await expect(page.locator("body")).toContainText("task 1");
    await page.locator("[data-cy=reload]").nth(2).click();
    await expect(page.locator("body")).toContainText("JOE");
});

test("should include tasks from the template file and override their env", async ({ page, wake }) => {
    const filePath = join(wake.dir, "tasks.inc");
    const includeContent = `
- name: Included task 1
  run: echo "task 1"

- name: Included task 2
  run: printenv HELLO_VAR
  env:
    HELLO_VAR: JOE
`;
    await writeFile(filePath, includeContent);
    const jobContent = `
desc: Include test
tasks:
  - name: Check parameters
    run: uname -a

  - include: ${filePath}
    env:
      HELLO_VAR: PATRICK
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(3);
    await page.locator("[data-cy=reload]").nth(1).click();
    await expect(page.locator("body")).toContainText("task 1");
    await page.locator("[data-cy=reload]").nth(2).click();
    await expect(page.locator("body")).toContainText("PATRICK");
});

test("should include tasks from the template file and preserve their env", async ({ page, wake }) => {
    const filePath = join(wake.dir, "tasks.inc");
    const includeContent = `
- name: Included task 1
  run: echo "task 1"

- name: Included task 2
  run: printenv HELLO_VAR
  env:
    HELLO_VAR: JOE
`;
    await writeFile(filePath, includeContent);
    const jobContent = `
desc: Include test
tasks:
  - name: Check parameters
    run: uname -a

  - include: ${filePath}
    env:
      TEST: 1
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(3);
    await page.locator("[data-cy=reload]").nth(1).click();
    await expect(page.locator("body")).toContainText("task 1");
    await page.locator("[data-cy=reload]").nth(2).click();
    await expect(page.locator("body")).toContainText("JOE");
});

test("should include tasks from the template file and use original when", async ({ page, wake }) => {
    const filePath = join(wake.dir, "tasks.inc");
    const includeContent = `
- name: Included task 1
  run: echo "task 1"
  when: $NAME == joe
`;
    await writeFile(filePath, includeContent);
    const jobContent = `
desc: Include test
params:
  - NAME: joe
tasks:
  - include: ${filePath}
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(1);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("task 1");
});

test("should include tasks from the template file and override original when", async ({ page, wake }) => {
    const filePath = join(wake.dir, "tasks.inc");
    const includeContent = `
- name: Included task 1
  run: echo "task 1"
  when: $NAME == joe
`;
    await writeFile(filePath, includeContent);
    const jobContent = `
desc: Include test
params:
  - NAME: tim
tasks:
  - include: ${filePath}
    when: $NAME == tim
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(1);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("task 1");
});

test("should include tasks from the template file and override original if", async ({ page, wake }) => {
    const filePath = join(wake.dir, "tasks.inc");
    const includeContent = `
- name: Included task 1
  run: echo "task 1"
  if: test $NAME == joe
`;
    await writeFile(filePath, includeContent);
    const jobContent = `
desc: Include test
params:
  - NAME: tim
tasks:
  - include: ${filePath}
    if: test $NAME == tim
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(1);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("task 1");
});
