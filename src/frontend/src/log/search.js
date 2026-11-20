import { prefixEnd } from "./ansi.js";

// The most matches a search keeps the places of; it still counts the rest
export const MAX_KEPT_MATCHES = 100000;

function escapeRegExp(text) {
    return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// lineMatches returns where query is in a line's text, after the time
// prefix, as [from, to] within the line: text, not a regex, ignoring case
// unless caseSensitive, as the server's grep matches
export function lineMatches(line, query, caseSensitive = false) {
    if (!query) {
        return [];
    }
    const re = new RegExp(escapeRegExp(query), caseSensitive ? "g" : "gi");
    re.lastIndex = prefixEnd(line);
    const found = [];
    for (let m = re.exec(line); m !== null; m = re.exec(line)) {
        found.push([m.index, m.index + m[0].length]);
    }
    return found;
}

// LogSearch holds the matches of a query in a log of lines numbered from 1.
// A line never changes once it is written, so new lines are searched on
// their own and the matches found before stay where they are.
export class LogSearch {
    constructor(query, caseSensitive = false) {
        this.query = query;
        this.caseSensitive = caseSensitive;
        // { line, from, to }, from and to within the line, in order
        this.matches = [];
        this.count = 0;
        // The lines with a match, in order, for "Only matching lines"
        this.lines = [];
    }

    // add searches lines, whose first is number first
    add(first, lines) {
        if (!this.query) {
            return;
        }
        lines.forEach((text, i) => {
            const found = lineMatches(text, this.query, this.caseSensitive);
            if (found.length === 0) {
                return;
            }
            const line = first + i;
            this.lines.push(line);
            this.count += found.length;
            for (const [from, to] of found) {
                if (this.matches.length < MAX_KEPT_MATCHES) {
                    this.matches.push({ line, from, to });
                }
            }
        });
    }

    // countText is "37" or, past the matches it keeps, "100,000+"
    countText() {
        const n = this.count.toLocaleString("en-US");
        return this.count > this.matches.length ? `${this.matches.length.toLocaleString("en-US")}+` : n;
    }

    // nearest returns the index of the first match at or after line, or the
    // last one
    nearest(line) {
        const i = this.matches.findIndex((m) => m.line >= line);
        return i < 0 ? this.matches.length - 1 : i;
    }
}
