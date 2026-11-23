// suggestedValues returns the values a build's summary suggests for a run
// again: each NAME=VALUE in it whose NAME is one of names, the job's params,
// as { NAME: value }, the last one winning. A test task writes, say, "Run
// only them again with TESTS=^(TestLogin)$" (configDescription.yaml).
export function suggestedValues(summary, names) {
    const found = {};
    if (!summary) {
        return found;
    }
    for (const m of summary.matchAll(/(?:^|\s)([A-Za-z_][A-Za-z0-9_]*)=(\S+)/g)) {
        if (names.includes(m[1])) {
            found[m[1]] = m[2];
        }
    }
    return found;
}
