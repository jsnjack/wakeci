const HTML_ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };

// escapeHtml makes a value safe to interpolate into notification text, which
// is rendered as raw HTML (see App.vue's dangerouslySetInnerHtml) so that
// notifications can include a clickable build link. Any value that isn't
// literal markup we authored ourselves - job names, API error bodies, build
// IDs - must go through this first.
export function escapeHtml(value) {
    return String(value).replace(/[&<>"']/g, (c) => HTML_ESCAPES[c]);
}

export function findInContainer(container, key, value) {
    for (let i = 0; i < container.length; i++) {
        if (container[i][key] === value) {
            return [container[i], i];
        }
    }
    return [undefined, undefined];
}

// filterMatchText is the text the server matches a search against
// (BuildUpdateData.ToFilterMatchString): id:<id> name:<job> status:<status>
// and the params as Go prints a []map[string]string, keys sorted:
// [map[NAME:value] map[OTHER:value]]. A different text here made the feed
// drop new builds that a search with id:, name:, status: or NAME:value
// matched on load.
export function filterMatchText(ev) {
    const params = (ev.params || []).map(
        (param) =>
            "map[" +
            Object.keys(param)
                .sort()
                .map((name) => `${name}:${param[name]}`)
                .join(" ") +
            "]",
    );
    return `id:${ev.id} name:${ev.name} status:${ev.status} [${params.join(" ")}]`;
}

export function isFilteredUpdate(ev, filterObj) {
    // Returns true if the update should be ignored
    // The filter is the same as on the server side (HandleFeedView)
    if (filterObj === null) {
        return false;
    }
    return !matchesFilter(ev, filterObj);
}

// The filter is similar to the implementation on the server side (HandleFeedView)
export function createFilterObj(query) {
    if (query.trim() === "") {
        return null;
    }

    // As unquote on the server: the same quote on both ends
    function unquote(term) {
        for (const quote of ['"', "'"]) {
            if (term.length >= 2 && term.startsWith(quote) && term.endsWith(quote)) {
                return term.slice(1, -1);
            }
        }
        return term;
    }

    // As SplitFilterQuery on the server: terms split at spaces outside
    // quotes, a quote ended only by the same quote, no empty terms
    function splitFilterQuery(query) {
        const terms = [];
        let current = "";
        let quote = "";
        for (const c of query) {
            if (c === '"' || c === "'") {
                if (!quote) {
                    quote = c;
                } else if (c === quote) {
                    quote = "";
                }
                current += c;
            } else if (c === " " && !quote) {
                if (current) {
                    terms.push(current);
                    current = "";
                }
            } else {
                current += c;
            }
        }
        if (current) {
            terms.push(current);
        }
        return terms;
    }

    let data = splitFilterQuery(query);
    let result = {
        MustExclude: [],
        MustInclude: [],
        ContainsAny: [],
    };

    for (let el of data) {
        if (el.startsWith("-")) {
            result.MustExclude.push(unquote(el.substring(1)));
        } else if (el.startsWith("+")) {
            result.MustInclude.push(unquote(el.substring(1)));
        } else {
            result.ContainsAny.push(unquote(el));
        }
    }

    return result;
}

// matchesFilter works like matchesFilter on the server: a term is found
// anywhere in the build's text, unless it is NAME=value (termMatches).
// e2e/search.spec.js checks both against the same cases.
export function matchesFilter(ev, filter) {
    if (filter === null) {
        return true;
    }
    const s = filterMatchText(ev);

    for (let item of filter.MustInclude) {
        if (!termMatches(ev, s, item)) {
            return false;
        }
    }

    for (let item of filter.MustExclude) {
        if (termMatches(ev, s, item)) {
            return false;
        }
    }

    let contains = filter.ContainsAny.length === 0;
    for (let item of filter.ContainsAny) {
        if (termMatches(ev, s, item)) {
            contains = true;
            break;
        }
    }

    return contains;
}

// LABEL_SUFFIX marks a param as a label when its job lists no labels
export const LABEL_SUFFIX = "_LABEL";

// shownName is the name a search finds a param by: lowercase, without
// _LABEL, so BRANCH_LABEL is branch
export function shownName(name) {
    const lower = name.toLowerCase();
    return lower.endsWith(LABEL_SUFFIX.toLowerCase()) ? lower.slice(0, -LABEL_SUFFIX.length) : lower;
}

// searchTerm is the term that finds builds whose param name is value,
// quoted only if it has a space or a quote. A name that NAME=value can't
// hold (EXACT_TERM), such as app.version, gives the value alone
export function searchTerm(name, value) {
    const shown = shownName(name);
    const term = /^[A-Za-z_][A-Za-z0-9_]*$/.test(shown) ? `${shown}=${value}` : value;
    if (!/[\s"']/.test(term)) {
        return term;
    }
    return term.includes('"') ? `'${term}'` : `"${term}"`;
}

// NAME=value matches a build whose id, name or status is value, or with a
// param equal to value named NAME, ignoring case, or NAME_LABEL: a label by
// its shown name, so branch=main finds BRANCH_LABEL
const EXACT_TERM = /^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/;

function termMatches(ev, text, term) {
    const m = EXACT_TERM.exec(term);
    if (m === null) {
        return text.includes(term);
    }
    const [, name, value] = m;
    const field = { id: String(ev.id), name: ev.name, status: ev.status }[name.toLowerCase()];
    if (field === value) {
        return true;
    }
    const wanted = [name.toLowerCase(), (name + LABEL_SUFFIX).toLowerCase()];
    // Else found as text, as other terms: a value can hold = too
    return (
        (ev.params || []).some((param) => Object.entries(param).some(([key, v]) => v === value && wanted.includes(key.toLowerCase()))) || text.includes(term)
    );
}

export function humanFileSize(size) {
    if (isNaN(parseInt(size, 10))) {
        return "";
    }
    const i = size === 0 ? 0 : Math.floor(Math.log(size) / Math.log(1024));
    return (size / Math.pow(1024, i)).toFixed(2) * 1 + " " + ["B", "kB", "MB", "GB", "TB"][i];
}

export function generateRandomString(length) {
    const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
    let result = "";
    for (let i = 0; i < length; i++) {
        result += characters.charAt(Math.floor(Math.random() * characters.length));
    }
    return result;
}

// matchJobPattern works like matchJobPattern on the server: * matches any run
// of characters and every other character only itself
export function matchJobPattern(pattern, name) {
    const parts = pattern.split("*");
    if (parts.length === 1) {
        return pattern === name;
    }
    if (!name.startsWith(parts[0])) {
        return false;
    }
    let rest = name.slice(parts[0].length);
    // Taking each middle part at its first match leaves the most room for the
    // ones after it
    for (const part of parts.slice(1, -1)) {
        const i = rest.indexOf(part);
        if (i < 0) {
            return false;
        }
        rest = rest.slice(i + part.length);
    }
    return rest.endsWith(parts[parts.length - 1]);
}

// grantHasScope mirrors Grant.HasScope in the backend: write only counts on
// a grant that covers all jobs
export function grantHasScope(grant, scope) {
    return grant.scopes.includes(scope) && (scope !== "write" || grant.allJobs);
}

// grantAllows mirrors Grant.HasScope and Grant.CoversJob in the backend
export function grantAllows(grant, scope, job) {
    return grantHasScope(grant, scope) && (grant.allJobs || grant.jobs.includes(job) || grant.patterns.some((pattern) => matchJobPattern(pattern, job)));
}

// askNotificationPermission asks for system notifications if the browser has
// them and the user hasn't answered; without them the page shows a message
export function askNotificationPermission() {
    if ("Notification" in window && Notification.permission === "default") {
        Notification.requestPermission().catch(() => {});
    }
}
