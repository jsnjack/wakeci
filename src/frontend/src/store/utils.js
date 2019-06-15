export function findInContainer(container, key, value) {
    for (let i=0; i<container.length; i++) {
        if (container[i][key] === value) {
            return [container[i], i];
        }
    }
    return [undefined, undefined];
}
