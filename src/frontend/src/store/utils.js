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

export function isFilteredUpdate(ev, filterObj) {
    // Returns true if the update should be ignored
    // The filter is similar to the implementation on the server side (HandleFeedView)
    if (filterObj === null) {
        return false;
    }
    const info = "" + ev.id + ev.name + ev.status + JSON.stringify(ev.params);
    return !matchesFilter(info, filterObj);
}

// The filter is similar to the implementation on the server side (HandleFeedView)
export function createFilterObj(query) {
    if (query.trim() === "") {
        return null;
    }

    function unquote(query) {
        if (/^["'].*["']$/.test(query)) {
            return query.slice(1, -1);
        }
        return query;
    }

    function splitFilterQuery(query) {
        let data = query.split(" ");
        let newData = handleOpenQuotes(data, '"');
        while (newData.length !== data.length) {
            data = newData;
            newData = handleOpenQuotes(data, '"');
        }
        newData = handleOpenQuotes(data, "'");
        while (newData.length !== data.length) {
            data = newData;
            newData = handleOpenQuotes(data, "'");
        }
        return data;
    }

    function handleOpenQuotes(data, quote) {
        for (let i = 0; i < data.length; i++) {
            let element = data[i];
            if (element.endsWith(quote)) {
                if (i > 0) {
                    let newData = [...data.slice(0, i - 1), data.slice(i - 1, i + 1).join(" "), ...data.slice(i + 1)];
                    return newData;
                }
            }
        }
        return data;
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

function matchesFilter(s, filter) {
    if (filter === null) {
        return true;
    }

    for (let item of filter.MustInclude) {
        if (!s.includes(item)) {
            return false;
        }
    }

    for (let item of filter.MustExclude) {
        if (s.includes(item)) {
            return false;
        }
    }

    let contains = filter.ContainsAny.length === 0;
    for (let item of filter.ContainsAny) {
        if (s.includes(item)) {
            contains = true;
            break;
        }
    }

    return contains;
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
