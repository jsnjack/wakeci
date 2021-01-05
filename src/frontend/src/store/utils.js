export function findInContainer(container, key, value) {
    for (let i=0; i<container.length; i++) {
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
