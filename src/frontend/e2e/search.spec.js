import { readFileSync } from "node:fs";
import { join } from "node:path";
import { test, expect } from "@playwright/test";
import { createFilterObj, matchesFilter } from "../src/store/utils.js";

// The browser's search parser against the cases the server's is tested with
// (query_parser_test.go), so the feed keeps live builds the server would show
const { builds, cases } = JSON.parse(readFileSync(join(import.meta.dirname, "../../backend/testdata/search_cases.json"), "utf8"));

test("the browser finds the builds the server finds", () => {
    for (const { query, finds } of cases) {
        const filter = createFilterObj(query);
        const found = builds.filter((build) => matchesFilter(build, filter)).map((build) => build.id);
        expect(found, query).toEqual(finds);
    }
});
