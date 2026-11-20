import { test, expect } from "@playwright/test";
import { hasTerm, quoteTerm, splitTerms, toggleTerms, withoutTerms } from "../src/feed.js";

// The terms of a feed search, which the quick filters write (F4, F8)
test("terms split at spaces outside quotes", () => {
    expect(splitTerms(`+status=failed  "status=timed out" -name=x 'a "b'`)).toEqual(["+status=failed", '"status=timed out"', "-name=x", `'a "b'`]);
});

test("filters add their terms, and take them out again", () => {
    let q = "branch=main";
    q = toggleTerms(q, ["+status=failed"]);
    expect(q).toBe("branch=main +status=failed");
    expect(hasTerm(q, "+status=failed")).toBe(true);
    q = toggleTerms(q, ["-status=finished", "-status=timed out"]);
    expect(q).toBe(`branch=main +status=failed -status=finished -"status=timed out"`);
    q = toggleTerms(q, ["-status=finished", "-status=timed out"]);
    expect(q).toBe("branch=main +status=failed");
    expect(withoutTerms(q, ["+status=failed"])).toBe("branch=main");
    expect(quoteTerm("+note=it's")).toBe(`+"note=it's"`);
});
