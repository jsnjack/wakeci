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


export function humanFileSize(size) {
    const i = size === 0 ? 0 : Math.floor( Math.log(size) / Math.log(1024) );
    return ( size / Math.pow(1024, i) ).toFixed(2) * 1 + " " + ["B", "kB", "MB", "GB", "TB"][i];
};
