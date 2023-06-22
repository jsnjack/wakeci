export function findInContainer(container, key, value) {
    for (let i = 0; i < container.length; i++) {
        if (container[i][key] === value) {
            return [container[i], i];
        }
    }
    return [undefined, undefined];
}

export function isFilteredUpdate(ev, filter) {
    // Returns true if the update should be ignored
    // The filter is similar to the implementation on the server side (HandleFeedView)
    if (filter === "") {
        return false;
    }
    const info = "" + ev.id + ev.name + ev.status + ev.params;
    return info.indexOf(filter) === -1;
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
