import { test, expect } from "@playwright/test";
import { groupStatus, groupTree, latestAttempts } from "../src/groups.js";

const build = (id, parent, name = "job", status = "finished", params = []) => ({ id, parent, name, status, params });

test("a group is a tree: each build's builds right under it, in the order they started", () => {
    const tree = groupTree(1, [build(1, 0), build(2, 1), build(3, 1), build(4, 2), build(5, 99)]);
    expect(tree.map((b) => [b.id, b.depth])).toEqual([
        [2, 1],
        [4, 2],
        [3, 1],
        // Its parent isn't shown
        [5, 1],
    ]);
});

test("only the latest attempt of a job with the same values shows, a matrix stays apart", () => {
    const shown = latestAttempts([
        build(2, 1, "tests", "failed"),
        build(3, 1, "lint"),
        build(4, 1, "tests"),
        build(5, 1, "shard", "finished", [{ N: "1" }]),
        build(6, 1, "shard", "finished", [{ N: "2" }]),
        build(7, 3, "tests"),
    ]);
    expect(shown.map((b) => [b.id, b.attempts])).toEqual([
        [3, 1],
        [4, 2],
        [5, 1],
        [6, 1],
        [7, 1],
    ]);
});

test("a group runs while any build runs, then shows the worst result", () => {
    expect(groupStatus([build(1, 0, "a", "finished"), build(2, 1, "b", "pending")])).toBe("running");
    expect(groupStatus([build(1, 0, "a", "aborted"), build(2, 1, "b", "failed")])).toBe("failed");
    expect(groupStatus([build(1, 0, "a", "finished"), build(2, 1, "b", "timed out")])).toBe("timed out");
    expect(groupStatus([build(1, 0, "a", "finished"), build(2, 1, "b", "skipped")])).toBe("finished");
});
