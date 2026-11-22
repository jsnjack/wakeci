// What the user asked to be notified about (L5 in docs/ui-roadmap.md):
// builds and searches, kept in the browser so they survive a reload, and
// checked on every page

const KEY = "notify";

export function loadWatches() {
    try {
        const saved = JSON.parse(window.localStorage.getItem(KEY) || "null");
        return {
            builds: Array.isArray(saved?.builds) ? saved.builds.filter(Number.isInteger) : [],
            searches: Array.isArray(saved?.searches) ? saved.searches.filter((q) => typeof q === "string" && q !== "") : [],
        };
    } catch {
        return { builds: [], searches: [] };
    }
}

export function saveWatches(watches) {
    try {
        window.localStorage.setItem(KEY, JSON.stringify(watches));
    } catch {
        // A browser that keeps nothing forgets them on reload
    }
}
