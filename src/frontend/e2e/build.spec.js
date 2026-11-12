import { test, expect } from "./fixtures.js";

test("should show build page", async ({ page, wake }) => {
    const jobName = await wake.createJob();
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("uname -a");
});

test("should inject wake env variables", async ({ page, wake }) => {
    const jobContent = `
desc: Test env variables
params:
  - pruzhany: 5
  - minsk: 4
tasks:
  - name: Print env
    run: env
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText(`WAKE_JOB_NAME=${jobName}`);
    await expect(page.locator("body")).toContainText(`WAKE_URL=${wake.url.replace("127.0.0.1", "localhost") + "/"}`);
    await expect(page.locator("body")).toContainText("WAKE_CONFIG_DIR=");
    await expect(page.locator("body")).toContainText("WAKE_BUILD_WORKSPACE=");
    await expect(page.locator("body")).toContainText("WAKE_BUILD_ID=");
    await expect(page.locator("body")).toContainText("WAKE_JOB_PARAMS=minsk=4&pruzhany=5");
});

test("should collect artifacts", async ({ page, wake }) => {
    const jobContent = `
desc: Test env variables
params:
  - pruzhany: 5
  - minsk: 4
tasks:
  - name: Create 1 file
    run: printf big > big

  - name: Create 2 file
    run: printf small > small

artifacts:
  - "*"
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page.locator("[data-cy=artifacts-body-row]")).toHaveCount(2);
    await expect(page.locator("[data-cy=artifacts-body-row] > td:first-child").first()).toContainText("big");
    await page.locator("[data-cy=artifacts-header-file]").click();
    await expect(page.locator("[data-cy=artifacts-body-row] > td:first-child").first()).toContainText("small");
    await page.locator("[data-cy=artifacts-header-file]").click();
    await expect(page.locator("[data-cy=artifacts-body-row] > td:first-child").first()).toContainText("big");
    await expect(page.locator("[data-cy=openIndexFile]")).toHaveCount(0);
});

test("should display Open index.html button", async ({ page, wake }) => {
    const jobContent = `
desc: Test env variables
params:
  - pruzhany: 5
  - minsk: 4
tasks:
  - name: Create 1 file
    run: mkdir -p bb && printf big > bb/index.html

  - name: Create 2 file
    run: mkdir -p aa/cc && printf small > aa/cc/index.html

artifacts:
  - "**"
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page.locator("[data-cy=openIndexFile]")).toContainText("index.html");
    await expect(page.locator("[data-cy=openIndexFile]")).toHaveAttribute("href", /bb/);
    await expect(page.locator("[data-cy=openIndexFile]")).toHaveAttribute("rel", "noopener noreferrer");
});

test("should skip task when condition is false", async ({ page, wake }) => {
    const jobContent = `
desc: Condition test
tasks:
  - name: Print env
    run: env
    when: 1 == 2
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("skipped");
    await expect(page.locator("body")).toContainText("Condition is false");
    await expect(page.locator("body")).not.toContainText("WAKE_BUILD_ID=");
});

test("should run task when condition is true", async ({ page, wake }) => {
    const jobContent = `
desc: Condition test
params:
 - NAME: joe
tasks:
  - name: Print env
    run: env
    when: $NAME == joe
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("Condition is true");
    await expect(page.locator("body")).toContainText("WAKE_BUILD_ID=");
});

test("should inject env variables", async ({ page, wake }) => {
    const jobContent = `
desc: Env test
tasks:
  - name: Print env
    run: env
    env:
      NAME: joe
      SCORE: 5
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("NAME=joe");
    await expect(page.locator("body")).toContainText("SCORE=5");
    await expect(page.locator("body")).toContainText("WAKE_BUILD_ID=");
});

test("should use build.env files", async ({ page, wake }) => {
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
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await expect(page.locator("[data-cy=reload]")).toHaveCount(4);
    await page.locator("[data-cy=reload]").nth(0).click();
    await expect(page.locator("body")).toContainText("This is bill");
    await page.locator("[data-cy=reload]").nth(1).click();
    await expect(page.locator("body")).toContainText("His best friend is joe");
    await page.locator("[data-cy=reload]").nth(3).click();
    await expect(page.locator("body")).toContainText("People call him big bill");
});

test("should skip task when `if` condition has non-zero exit code", async ({ page, wake }) => {
    const jobContent = `
desc: Condition test
tasks:
  - name: Print env
    run: env
    if: test -n ""
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("skipped");
    await expect(page.locator("body")).toContainText("Condition is false");
    await expect(page.locator("body")).not.toContainText("WAKE_BUILD_ID=");
});

test("should run task when `if` condition has exit code 0", async ({ page, wake }) => {
    const jobContent = `
desc: Condition test
params:
 - NAME: joe
tasks:
  - name: Print env
    run: env
    if: test -n "$NAME"
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("Condition is true");
    await expect(page.locator("body")).toContainText("WAKE_BUILD_ID=");
});

test("should not run task when `if` condition has exit code 0 and `when` condition fails", async ({ page, wake }) => {
    const jobContent = `
desc: Condition test
params:
 - NAME: joe
tasks:
  - name: Print env
    run: env
    if: test -n "$NAME"
    when: $NAME = bill
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("Skipping the task");
    await expect(page.locator("body")).not.toContainText("WAKE_BUILD_ID=");
});

test("should not run task when `if` condition has exit code 1 and `when` condition passes", async ({ page, wake }) => {
    const jobContent = `
desc: Condition test
params:
 - NAME: joe
tasks:
  - name: Print env
    run: env
    if: test -n "$NOT"
    when: $NAME = joe
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("Skipping the task");
    await expect(page.locator("body")).not.toContainText("WAKE_BUILD_ID=");
});

test("should run task when `if` condition has exit code 0 and `when` condition passes", async ({ page, wake }) => {
    const jobContent = `
desc: Condition test
params:
 - NAME: joe
tasks:
  - name: Print env
    run: env
    if: test -n "$NAME"
    when: $NAME = joe
`;
    const jobName = await wake.createJob(jobContent);
    const id = await wake.runJob(jobName);
    await wake.openBuild(id);
    await expect(page).toHaveURL(`${wake.url}/build/${id}`);
    await page.locator("[data-cy=reload]").click();
    await expect(page.locator("body")).toContainText("Condition is true");
    await expect(page.locator("body")).not.toContainText("Skipping the task");
    await expect(page.locator("body")).toContainText("WAKE_BUILD_ID=");
});

test("should show the summary a job writes when the build ends", async ({ page, wake }) => {
    const jobName = await wake.createJob(`desc: summary
tasks:
  - name: Test
    run: sleep 3
  - name: Write the summary
    run: |
      echo "### Failed tests" >> "$WAKE_SUMMARY"
      echo "- test_login <b>" >> "$WAKE_SUMMARY"
`);
    const id = await wake.runJob(jobName);
    await wake.waitForBuild(id, "running");
    // The page is open while the build runs, so the summary has to come
    // from the reload when the build ends
    await wake.livePage(`/build/${id}`);
    await expect(page.locator("header h6")).toContainText(`#${id} - running`);
    await expect(page.getByTestId("build-summary")).toHaveCount(0);
    // Shown as text, so markup in it stays text
    await expect(page.getByTestId("build-summary")).toContainText("### Failed tests\n- test_login <b>", { timeout: 15000 });
    await wake.waitForBuild(id);
});
