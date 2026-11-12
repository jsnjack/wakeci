// Helpers of the analytics pages: build statuses, durations, rates and dates
// as the pages show them. Durations come from the API in nanoseconds.
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

// addLocale, unlike addDefaultLocale, may be called by every module that
// needs it
TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

// The final statuses, in the order the charts stack them from the baseline:
// failures first, so they line up and compare across days. key is the field
// of the API's counts, cls the class that colours a mark
export const STATUSES = [
    { key: "failed", status: "failed", label: "failed", icon: "cancel", cls: "s-failed" },
    { key: "timedOut", status: "timed out", label: "timed out", icon: "acute", cls: "s-timed-out" },
    { key: "aborted", status: "aborted", label: "aborted", icon: "do_not_disturb_on", cls: "s-aborted" },
    { key: "finished", status: "finished", label: "finished", icon: "check_circle", cls: "s-finished" },
];

export function statusOf(status) {
    return STATUSES.find((s) => s.status === status) || { key: status, status, label: status, icon: "pending", cls: "s-pending" };
}

// Who or what starts builds, by the API's trigger kind
export const TRIGGERS = {
    interval: "Schedule (interval)",
    password: "Admin password",
    sso: "Single sign-on",
    apikey: "API keys",
    unknown: "Not recorded (older builds)",
};

const NS = 1e9;

// formatDuration writes nanoseconds as "45s", "5m 07s" or "1h 05m"
export function formatDuration(ns) {
    const s = Math.round(ns / NS);
    if (s < 60) {
        return `${s}s`;
    }
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (h > 0) {
        return `${h}h ${String(m).padStart(2, "0")}m`;
    }
    return `${m}m ${String(s % 60).padStart(2, "0")}s`;
}

// axisDuration writes nanoseconds briefly for an axis: "30s", "5m", "2h"
export function axisDuration(ns) {
    const s = ns / NS;
    if (s < 60) {
        return `${Math.round(s)}s`;
    }
    if (s < 3600) {
        return `${Math.round(s / 60)}m`;
    }
    return `${Math.round((s / 3600) * 10) / 10}h`;
}

// niceDurationMax rounds a duration up to a value with round axis labels
export function niceDurationMax(ns) {
    const steps = [10, 30, 60, 120, 300, 600, 900, 1200, 1800, 3600, 7200, 10800, 21600, 43200, 86400].map((s) => s * NS);
    return steps.find((step) => step >= ns) || Math.ceil(ns / (86400 * NS)) * 86400 * NS;
}

// niceMax rounds a count up to 1, 2, 2.5 or 5 times a power of ten
export function niceMax(value) {
    if (value <= 0) {
        return 1;
    }
    const power = Math.pow(10, Math.floor(Math.log10(value)));
    return [1, 2, 2.5, 5, 10].map((m) => m * power).find((m) => m >= value);
}

// formatRate writes a rate from 0 to 1 as a percentage, "—" if there is none
export function formatRate(rate, digits = 0) {
    if (rate === null || rate === undefined) {
        return "—";
    }
    return `${(rate * 100).toFixed(digits)}%`;
}

// isZeroTime reports whether the API sent Go's zero time, which means unknown
export function isZeroTime(value) {
    return !value || value.startsWith("0001-");
}

export function ago(value) {
    if (isZeroTime(value)) {
        return "";
    }
    return timeAgo.format(new Date(value));
}

export function dateTime(value) {
    if (isZeroTime(value)) {
        return "";
    }
    return new Date(value).toLocaleString();
}

// dayLabel writes a day of the API, 2026-11-12, as "Nov 12"
export function dayLabel(day, withYear = false) {
    const options = { month: "short", day: "numeric", timeZone: "UTC" };
    if (withYear) {
        options.year = "numeric";
    }
    return new Date(`${day}T00:00:00Z`).toLocaleDateString(undefined, options);
}

let ids = 0;

// uniqueID names an element, such as a chart's pattern, apart from those of
// other charts on the page
export function uniqueID(prefix) {
    ids++;
    return `${prefix}-${ids}`;
}
