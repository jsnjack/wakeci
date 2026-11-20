import { LABEL_SUFFIX } from "./store/utils.js";

// How many values a feed row shows; the build page lists them all
export const MAX_ROW_VALUES = 5;

// Values longer than this are cut in the middle
const MAX_VALUE_LENGTH = 32;

// rowValues returns what a feed row shows of a build's params, as
// { name, value, short }: the labels first, in the job's order, then the
// other params the build changed. Empty values and values at the job's
// default say nothing about the build and are left out, labels too. job is
// { defaults: { NAME: value }, labels: [NAME] }, or null while unknown:
// then every value counts as changed. Without labels, the *_LABEL params
// are the labels, as on the server (Job.labelNames).
export function rowValues(params, job) {
    const defaults = job?.defaults || {};
    const labels = job?.labels?.length ? job.labels : (params || []).map((param) => Object.keys(param)[0]).filter((name) => name.endsWith(LABEL_SUFFIX));
    const shown = [];
    for (const param of params || []) {
        const [name, value] = Object.entries(param)[0] || [];
        if (name === undefined || value === "" || (Object.hasOwn(defaults, name) && defaults[name] === value)) {
            continue;
        }
        shown.push({ name, value, short: shortValue(value) });
    }
    const rank = (name) => {
        const i = labels.indexOf(name);
        return i < 0 ? labels.length : i;
    };
    // sort is stable, so the other params keep their order
    return shown.sort((a, b) => rank(a.name) - rank(b.name));
}

// shortValue shortens a commit hash to 7 characters, and cuts a long value
// in the middle, keeping a leading ticket number such as T-4821
export function shortValue(value) {
    if (/^[0-9a-f]{40}$/i.test(value)) {
        return value.slice(0, 7);
    }
    if (value.length <= MAX_VALUE_LENGTH) {
        return value;
    }
    const ticket = /^([A-Za-z]+-\d+|#?\d+)/.exec(value);
    const head = ticket ? ticket[0] : value.slice(0, 14);
    return `${head}…${value.slice(-(MAX_VALUE_LENGTH - head.length - 1))}`;
}

// ordinal writes 1st, 2nd, 3rd, 4th, 11th, 21st…
export function ordinal(n) {
    const tens = n % 100;
    if (tens >= 11 && tens <= 13) {
        return `${n}th`;
    }
    return n + ({ 1: "st", 2: "nd", 3: "rd" }[n % 10] || "th");
}

// splitTerms splits a search into its terms as the server does
// (SplitFilterQuery): at spaces outside quotes, keeping the quotes
export function splitTerms(query) {
    const terms = [];
    let current = "";
    let quote = null;
    for (const ch of query) {
        if ((ch === '"' || ch === "'") && (quote === null || quote === ch)) {
            quote = quote === null ? ch : null;
            current += ch;
        } else if (ch === " " && quote === null) {
            if (current) {
                terms.push(current);
            }
            current = "";
        } else {
            current += ch;
        }
    }
    if (current) {
        terms.push(current);
    }
    return terms;
}

// termText is a term without its quotes: +"a b" is +a b
function termText(term) {
    const sign = /^[+-]/.test(term) ? term[0] : "";
    const rest = term.slice(sign.length).replace(/^(["'])(.*)\1$/, "$2");
    return sign + rest;
}

// quoteTerm writes a term with quotes if it has a space or a quote, after
// its sign: +"status=timed out"
export function quoteTerm(term) {
    const sign = /^[+-]/.test(term) ? term[0] : "";
    const rest = term.slice(sign.length);
    if (!/[\s"']/.test(rest)) {
        return term;
    }
    return sign + (rest.includes('"') ? `'${rest}'` : `"${rest}"`);
}

export function hasTerm(query, term) {
    return splitTerms(query).some((t) => termText(t) === termText(term));
}

// withoutTerms leaves the given terms out of a search
export function withoutTerms(query, terms) {
    const drop = terms.map(termText);
    return splitTerms(query)
        .filter((t) => !drop.includes(termText(t)))
        .join(" ");
}

// toggleTerms adds the terms to a search, or takes them out if it has them
// all
export function toggleTerms(query, terms) {
    if (terms.length && terms.every((term) => hasTerm(query, term))) {
        return withoutTerms(query, terms);
    }
    const missing = terms.filter((term) => !hasTerm(query, term)).map(quoteTerm);
    return [query.trim(), ...missing].filter(Boolean).join(" ");
}

// The statuses of builds that ended, which "Running and queued" leaves out
export const DONE_STATUSES = ["finished", "failed", "aborted", "timed out", "skipped"];
