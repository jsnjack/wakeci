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

// ERROR_RE finds the words that usually say why a task failed: test
// failures, errors and exceptions of common tools, a traceback or a panic.
// Case matters, so "errors: 0" and "no failures" don't count.
const ERROR_RE = new RegExp(
    [
        String.raw`\b(?:FAIL(?:ED|URE)?|ERROR|FATAL|PANIC)\b`,
        String.raw`\b[A-Z]\w*(?:Error|Exception)\b`,
        String.raw`\b(?:Error|Exception)\b`,
        String.raw`\berror(?:\[\w+\])?:`,
        String.raw`\bTraceback \(most recent call last\)`,
        String.raw`\bpanic:`,
        String.raw`\bfailed!`,
        String.raw`npm ERR!`,
        "[✕✗✘]",
    ].join("|"),
    "g",
);

// wakeci's own lines when a task ended badly: an exit code but 0, a time-out
// or an abort
const ENDED_BADLY_RE = /^> (?:Exit code: [1-9]\d*$|Timed out\.|Aborted)/;

// errorMatches returns where a line says something failed, after the time
// prefix, as lineMatches does; wakeci's own line about a failed exit is
// matched whole
export function errorMatches(line) {
    const start = prefixEnd(line);
    if (ENDED_BADLY_RE.test(line.slice(start))) {
        return [[start, line.length]];
    }
    ERROR_RE.lastIndex = start;
    const found = [];
    for (let m = ERROR_RE.exec(line); m !== null; m = ERROR_RE.exec(line)) {
        found.push([m.index, m.index + m[0].length]);
    }
    return found;
}

// LogSearch holds the matches of a query, or of errorMatches with errors, in
// a log of lines numbered from 1. A line never changes once it is written,
// so new lines are searched on their own and the matches found before stay
// where they are.
export class LogSearch {
    constructor(query, caseSensitive = false, errors = false) {
        this.query = errors ? "" : query;
        this.caseSensitive = caseSensitive;
        this.errors = errors;
        // { line, from, to }, from and to within the line, in order
        this.matches = [];
        this.count = 0;
        // The lines with a match, in order, for "Only matching lines"
        this.lines = [];
    }

    // active is true when there is something to look for
    get active() {
        return this.errors || this.query !== "";
    }

    // find returns where the search matches a line's text
    find(text) {
        return this.errors ? errorMatches(text) : lineMatches(text, this.query, this.caseSensitive);
    }

    // add searches lines, whose first is number first
    add(first, lines) {
        if (!this.active) {
            return;
        }
        lines.forEach((text, i) => {
            const found = this.find(text);
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
