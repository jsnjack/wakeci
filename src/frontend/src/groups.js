// Builds started by other builds (F2 in docs/ui-roadmap.md), shared by the
// feed and the build page

// Levels shown before a deeper build says "via #N" instead
export const MAX_DEPTH = 4;

const ACTIVE = ["pending", "running"];
// From worst to best, for the status of an ended group
const WORST_FIRST = ["failed", "timed out", "aborted", "finished", "skipped"];

// groupTree returns the builds under root as a tree: each build's builds
// right under it, in the order they started, each with its level, 1 for
// those the root started. A build whose parent isn't among builds sits at
// level 1
export function groupTree(root, builds) {
    const shown = new Set(builds.map((build) => build.id));
    const children = new Map();
    for (const build of builds) {
        if (build.id === root) {
            continue;
        }
        const parent = shown.has(build.parent) ? build.parent : root;
        children.set(parent, [...(children.get(parent) || []), build]);
    }
    const tree = [];
    const walk = (id, depth) => {
        for (const build of children.get(id) || []) {
            tree.push({ ...build, depth });
            walk(build.id, depth + 1);
        }
    };
    walk(root, 1);
    return tree;
}

// latestAttempts keeps the latest of the builds that one parent started of
// the same job with the same values, with how many attempts it took. Builds
// of a job with other values, e.g. a test matrix, stay apart
export function latestAttempts(builds) {
    const key = (build) => JSON.stringify([build.parent || 0, build.name, build.params || []]);
    const latest = new Map();
    for (const build of builds) {
        latest.set(key(build), { ...build, attempts: (latest.get(key(build))?.attempts || 0) + 1 });
    }
    return builds.filter((build) => latest.get(key(build)).id === build.id).map((build) => latest.get(key(build)));
}

// groupStatus is running while any build of the group is pending or
// running, and then the worst status among them
export function groupStatus(builds) {
    if (builds.some((build) => ACTIVE.includes(build.status))) {
        return "running";
    }
    for (const status of WORST_FIRST) {
        if (builds.some((build) => build.status === status)) {
            return status;
        }
    }
    return builds[0]?.status || "pending";
}

export function isFailure(build) {
    return build.status === "failed" || build.status === "timed out";
}
