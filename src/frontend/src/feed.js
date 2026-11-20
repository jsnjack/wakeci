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
// then every value counts as changed and the *_LABEL params are the labels.
export function rowValues(params, job) {
    const defaults = job?.defaults || {};
    const labels = job?.labels || (params || []).map((param) => Object.keys(param)[0]).filter((name) => name.endsWith(LABEL_SUFFIX));
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
