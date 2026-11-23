import { test, expect } from "@playwright/test";
import { LogSearch, errorMatches, lineMatches } from "../src/log/search.js";

// The log viewer's search (B4)
test("matches text after the time prefix, ignoring case", () => {
    expect(lineMatches("[    1.200s] GET /cart 200 OK", "200")).toEqual([[23, 26]]);
    expect(lineMatches("[ 0.1s] Timeout, timeout", "timeout")).toEqual([
        [8, 15],
        [17, 24],
    ]);
    expect(lineMatches("[ 0.1s] Timeout", "timeout", true)).toEqual([]);
    expect(lineMatches("[ 0.1s] a.b", ".")).toEqual([[9, 10]]);
});

test("new lines add their matches, and the count grows", () => {
    const search = new LogSearch("fail");
    search.add(1, ["[ 0.1s] ok", "[ 0.2s] FAIL one", "[ 0.3s] fail, fail"]);
    expect(search.count).toBe(3);
    expect(search.lines).toEqual([2, 3]);
    search.add(4, ["[ 0.4s] FAIL two"]);
    expect(search.countText()).toBe("4");
    expect(search.matches.map((m) => m.line)).toEqual([2, 3, 3, 4]);
    expect(search.nearest(3)).toBe(1);
    expect(search.nearest(9)).toBe(3);
});

test("the Errors search finds what usually says a task failed", () => {
    const at = (line) => errorMatches(line).map(([from, to]) => line.slice(from, to));
    expect(at("[ 0.1s] --- FAIL: TestLogin (0.01s)")).toEqual(["FAIL"]);
    expect(at("[ 0.1s] FAILED tests/test_cart.py::test_total - AssertionError")).toEqual(["FAILED", "AssertionError"]);
    expect(at("[ 0.1s] Traceback (most recent call last):")).toEqual(["Traceback (most recent call last)"]);
    expect(at("[ 0.1s] main.go:12:3: error: undefined: x")).toEqual(["error:"]);
    expect(at("[ 0.1s] error[E0425]: cannot find value")).toEqual(["error[E0425]:"]);
    expect(at("[ 0.1s] panic: runtime error: index out of range")).toEqual(["panic:", "error:"]);
    expect(at("[ 0.1s] npm ERR! code ELIFECYCLE")).toEqual(["npm ERR!"]);
    expect(at("[ 0.1s]   ✘ checkout › pays (1.2s)")).toEqual(["✘"]);
    expect(at("[ 0.1s] > Exit code: 2")).toEqual(["> Exit code: 2"]);
    expect(at("[ 0.1s] > Timed out.")).toEqual(["> Timed out."]);
    // Not failures
    for (const line of [
        "[ 0.1s] > Exit code: 0",
        "[ 0.1s] 12 passed, 0 errors",
        "[ 0.1s] no failures",
        "[ 0.1s] ERRORS=0 in the config? fine",
        "[ 0.1s] terror: a word",
    ]) {
        expect(at(line), line).toEqual([]);
    }
});

test("an Errors search ignores the query and counts new lines", () => {
    const search = new LogSearch("ok", false, true);
    expect(search.active).toBe(true);
    search.add(1, ["[ 0.1s] ok", "[ 0.2s] --- FAIL: x", "[ 0.3s] > Exit code: 1"]);
    expect(search.lines).toEqual([2, 3]);
    expect(new LogSearch("", false, false).active).toBe(false);
});
