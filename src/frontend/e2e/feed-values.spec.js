import { test, expect } from "@playwright/test";
import { ordinal, rowValues, runValues, shortValue } from "../src/feed.js";
import { searchTerm } from "../src/store/utils.js";

// What a feed row shows of a build's params (F1)
test("labels come first, then the other changed params", () => {
    const params = [{ COMMIT: "abc" }, { TARGET: "prod" }, { AUTHOR_LABEL: "alice" }, { BRANCH_LABEL: "main" }, { SUITE: "" }];
    const job = { defaults: { TARGET: "staging", BRANCH_LABEL: "unknown" }, labels: ["BRANCH_LABEL", "AUTHOR_LABEL"] };
    expect(rowValues(params, job).map((v) => v.name)).toEqual(["BRANCH_LABEL", "AUTHOR_LABEL", "COMMIT", "TARGET"]);
});

test("values at the job's default are left out, labels too", () => {
    const params = [{ BRANCH_LABEL: "unknown" }, { TARGET: "staging" }, { NOTE: "x" }];
    const job = { defaults: { BRANCH_LABEL: "unknown", TARGET: "staging" }, labels: ["BRANCH_LABEL"] };
    expect(rowValues(params, job).map((v) => v.name)).toEqual(["NOTE"]);
});

test("without the job, every value counts and *_LABEL params are the labels", () => {
    const params = [{ TARGET: "prod" }, { BRANCH_LABEL: "main" }];
    expect(rowValues(params, null).map((v) => v.name)).toEqual(["BRANCH_LABEL", "TARGET"]);
    expect(rowValues(null, null)).toEqual([]);
});

test("long values are shortened", () => {
    expect(shortValue("3f9c2a1e5d4b8a0c7f6e2d1b9a8c7f6e5d4b3a2c")).toBe("3f9c2a1");
    expect(shortValue("main")).toBe("main");
    expect(shortValue("T-4821-fix-the-login-redirect-after-an-sso-timeout")).toBe("T-4821…rect-after-an-sso-timeout");
    expect(shortValue("feature/a-branch-name-without-a-ticket-number")).toBe("feature/a-bran…t-a-ticket-number");
});

test("places in the queue read as ordinals", () => {
    expect([1, 2, 3, 4, 11, 12, 13, 21, 22, 101].map(ordinal)).toEqual(["1st", "2nd", "3rd", "4th", "11th", "12th", "13th", "21st", "22nd", "101st"]);
});

// Running a build again leaves out its values at the job's defaults, so one
// who may not edit jobs isn't refused a default that holds a quote
test("running again sends only values that differ from the job's defaults", () => {
    const params = [{ MSG: "it's ok" }, { TARGET: "prod" }, { NEW: "x" }];
    expect(runValues(params, { MSG: "it's ok", TARGET: "staging" })).toEqual([
        ["TARGET", "prod"],
        ["NEW", "x"],
    ]);
    // Defaults not known yet: every value
    expect(runValues(params, null)).toHaveLength(3);
});

// A chip's search term: NAME=value, or the value alone for a name that
// NAME=value can't hold
test("a value chip searches name=value, or the value for an unusual name", () => {
    expect(searchTerm("BRANCH_LABEL", "main")).toBe("branch=main");
    expect(searchTerm("app.version", "1.2")).toBe("1.2");
    expect(searchTerm("TARGET", "a b")).toBe('"target=a b"');
});
