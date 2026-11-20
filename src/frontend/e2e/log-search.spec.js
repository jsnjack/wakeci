import { test, expect } from "@playwright/test";
import { LogSearch, lineMatches } from "../src/log/search.js";

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
