import { test, expect } from "./fixtures.js";
import { suggestedValues } from "../src/summary.js";

test("a summary suggests the values of the job's params only", () => {
    expect(suggestedValues("Failed: TestLogin\nRun only them again with TESTS=^(TestLogin)$", ["TESTS", "TARGET"])).toEqual({ TESTS: "^(TestLogin)$" });
    expect(suggestedValues("NOT_A_PARAM=1 TARGET=a TARGET=b", ["TARGET"])).toEqual({ TARGET: "b" });
    expect(suggestedValues("x=TARGET=c", ["TARGET"])).toEqual({});
    expect(suggestedValues("", ["TARGET"])).toEqual({});
});

test("Run again offers the values the summary suggests", async ({ page, wake, backend }) => {
    const name = await wake.createJob(
        'params:\n  - TESTS: ""\ntasks:\n  - name: test\n    run: |\n' +
            '      echo "Run only them again with TESTS=^(TestLogin)$" >> "$WAKE_SUMMARY"\n' +
            '      test -n "$TESTS"\n',
    );
    const id = await wake.runJob(name);
    await wake.openBuild(id, "failed");
    await page.getByTestId("run-again-button").click();
    await expect(page.getByTestId("run-suggested")).toContainText("TESTS=^(TestLogin)$");
    await page.getByTestId("run-use-suggested").click();
    await expect(page.getByTestId("run-suggested")).toHaveCount(0);
    const scheduled = page.waitForResponse((response) => new URL(response.url()).pathname === `/api/job/${name}/run`);
    await page.getByTestId("start-job-confirm").click();
    const again = await (await scheduled).json();
    await wake.waitForBuild(again);
    const params = (await (await backend.api.get(`/api/build/${again}`)).json()).status_update.params;
    expect(Object.assign({}, ...params).TESTS).toBe("^(TestLogin)$");
});
