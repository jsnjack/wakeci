<template>
    <div
        class="ui feed-page"
        :class="{ split }"
    >
        <div class="feed-column stack gap-3">
            <FeedSearch
                v-model="filter"
                :jobs="jobs"
                :builds="builds"
                :fetching="isFetching"
                @clear="clearFilter"
            />

            <div
                v-if="filteredUpdates !== 0"
                class="panel cluster between text-sm"
                data-cy="filtered-updates"
            >
                <span>{{ filteredUpdates }} new {{ filteredUpdates === 1 ? "build doesn't" : "builds don't" }} match the search</span>
                <button
                    type="button"
                    class="btn btn-sm"
                    @click="clearFilter"
                >
                    Clear the search
                </button>
            </div>

            <div
                class="feed-list"
                data-cy="feed-container"
            >
                <template
                    v-for="item in sortedBuilds"
                    :key="item.id"
                >
                    <FeedItem
                        :build="item"
                        :job="jobs.get(item.name) || null"
                        :now="now"
                        :selectable="wide"
                        :selected="item.id === selected"
                        :compact="split"
                        @set-filter="handleSetFilter"
                        @select="selectBuild"
                    />
                    <FeedGroup
                        v-if="item.members?.length || item.hiddenMembers"
                        :root="item"
                        :jobs="jobs"
                        :selectable="wide"
                        :selected="selected"
                        @select="selectBuild"
                    />
                </template>
            </div>

            <div
                v-if="sortedBuilds.length === 0 && !isFetching && !filterIsDirty"
                class="feed-empty stack muted"
                data-cy="no-builds-found"
            >
                <i class="icon large">water</i>
                <span>No builds found</span>
            </div>

            <div
                v-show="moreEnabled && !isFetching && !filterIsDirty"
                class="cluster end"
            >
                <button
                    type="button"
                    class="btn"
                    data-cy="more"
                    @click="fetchNow(true)"
                >
                    Load older builds
                </button>
            </div>
        </div>
        <aside
            v-if="split"
            class="feed-pane stack gap-2"
            data-cy="build-pane"
            :aria-label="`Build ${selected}`"
        >
            <div class="cluster end gap-1">
                <router-link
                    :to="`/build/${selected}`"
                    class="btn btn-ghost btn-sm"
                    data-cy="pane-open-page"
                >
                    <i class="icon small">open_in_full</i> Open as a page
                </router-link>
                <button
                    type="button"
                    class="btn btn-ghost btn-icon btn-sm tip-end"
                    aria-label="Close"
                    data-tip="Close (Esc)"
                    data-cy="pane-close"
                    @click="closeBuild"
                >
                    <i class="icon small">close</i>
                </button>
            </div>
            <BuildView
                :id="selected"
                :key="selected"
                embedded
            />
        </aside>
    </div>
</template>

<script>
import FeedGroup from "@/components/FeedGroup.vue";
import FeedItem from "@/components/FeedItem.vue";
import FeedSearch from "@/components/FeedSearch.vue";
import { defineAsyncComponent } from "vue";
import { createFilterObj, findInContainer, isFilteredUpdate } from "@/store/utils.js";
import axios from "axios";
import debounce from "lodash/debounce";
import vuex from "vuex";

const FetchItemsSize = 10;

// From this width a click on a row shows the build beside the feed (F9);
// below, it opens the build's page
const WIDE = "(min-width: 1440px)";

export default {
    components: {
        FeedGroup,
        FeedItem,
        FeedSearch,
        // Loaded with the first build shown beside the feed
        BuildView: defineAsyncComponent(() => import("@/views/BuildView.vue")),
    },
    data: function () {
        return {
            builds: [],
            subscription: "build:update:",
            isFetching: false, // request to the server is in progress
            filterIsDirty: false, // when user is still typing
            filter: "", // sent to the server, to filter builds out
            moreEnabled: true, // if makes sense to load more builds from the server
            filteredUpdates: 0, // When `filter` is active, updates which do not much are counted here
            filterObj: null,
            // Each job's { defaults: { NAME: value }, labels: [NAME],
            // interval }, so a row can show the labels first and leave
            // defaults out, and the search can suggest and filter
            jobs: new Map(),
            // The time the rows show durations against, moved on every second
            now: Date.now(),
            // The build shown beside the feed (F9), from the address's build
            selected: null,
            // Whether the window is wide enough for it
            wide: false,
        };
    },
    computed: {
        ...vuex.mapState(["ws"]),
        split() {
            return this.wide && this.selected !== null;
        },
        sortedBuilds: function () {
            return [...this.builds].sort((a, b) => {
                if (a.id < b.id) {
                    return 1;
                }
                if (a.id > b.id) {
                    return -1;
                }
                return 0;
            });
        },
    },
    watch: {
        filter: function (value) {
            this.filterIsDirty = true;
            this.filterObj = createFilterObj(value);
            // Reset builds if user starts to change filter
            this.builds = [];
            this.showFilterInURL();
            this.fetch();
        },
        "ws.connected": "onWSChange",
        "$route.query.build": "readSelected",
        // Builds of jobs the user may now read, and those sent while
        // disconnected. Not fetchNow itself: it would take the count for
        // `more` and load older builds instead.
        "ws.reconnects"() {
            this.fetchNow();
            this.fetchJobs();
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Feed");

        // Restore filter from URL in address bar. Links written before
        // wakeci encoded it once carry it in `filter`, encoded twice
        // (status%253Afailed), so that one is decoded once more; showFilterInURL
        // then moves it to `q`
        const url = new URL(window.location.href);
        if (url.searchParams.has("q")) {
            this.filter = url.searchParams.get("q");
        } else if (url.searchParams.has("filter")) {
            const filterValue = url.searchParams.get("filter");
            try {
                this.filter = decodeURIComponent(filterValue);
            } catch {
                this.filter = filterValue;
            }
        }

        this.fetchNow();
        this.fetchJobs();
        this.subscribe();
        this.emitter.on(this.subscription, this.applyUpdate);
        this.clock = setInterval(() => (this.now = Date.now()), 1000);

        this.wideQuery = window.matchMedia(WIDE);
        this.wide = this.wideQuery.matches;
        this.wideQuery.addEventListener("change", this.onWideChange);
        document.addEventListener("keydown", this.onKeyDown);
        this.readSelected();
        // A shared address with a build opens its page where the window is
        // too narrow to show it beside the feed
        if (this.selected !== null && !this.wide) {
            this.$router.replace(`/build/${this.selected}`);
        }
    },
    unmounted() {
        this.unsubscribe();
        this.emitter.off(this.subscription, this.applyUpdate);
        clearInterval(this.clock);
        this.wideQuery.removeEventListener("change", this.onWideChange);
        document.removeEventListener("keydown", this.onKeyDown);
    },
    created() {
        this.fetch = debounce((more = false) => {
            this.isFetching = true;
            // More builds are those older than the oldest one shown. Unlike
            // skipping as many builds as are shown, that can't repeat or miss
            // builds when the page and the server filter one differently
            let before = "";
            if (more && this.builds.length > 0) {
                before = `before=${Math.min(...this.builds.map((build) => build.id))}&`;
            }
            axios
                .get(`/api/feed?groups=1&${before}q=${encodeURIComponent(this.filter)}`)
                .then((response) => {
                    this.isFetching = false;

                    const data = response.data || [];
                    data.forEach((element) => {
                        this.applyUpdate(element, true);
                    });
                    if (data.length < FetchItemsSize) {
                        // Server returned less than pageSize, so no more builds
                        // available
                        this.moreEnabled = false;
                    } else {
                        this.moreEnabled = true;
                    }
                })
                .catch(() => {});
            this.filterIsDirty = false;
        }, 500);
    },
    methods: {
        // Puts the filter in the address bar to allow copying and link
        // sharing. As the user types, not when the feed answers: by then the
        // user may be opening another page, and vue-router takes a replace
        // for a navigation that cancels one still loading its page
        showFilterInURL() {
            const url = new URL(window.location.href);
            // searchParams encodes the value itself. `q`, not `filter`, so
            // that a link tells which encoding it has (mounted)
            url.searchParams.delete("filter");
            if (this.filter === "") {
                url.searchParams.delete("q");
            } else {
                url.searchParams.set("q", this.filter);
            }
            // Not for the filter restored from the URL: a replace by the same
            // URL would count as the user's newer navigation (router.js)
            const target = url.pathname + url.search;
            if (this.$router.resolve(target).fullPath !== this.$route.fullPath) {
                this.$router.replace(target);
            }
        },
        subscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:subscribe",
                data: {
                    to: [this.subscription],
                },
            });
        },
        unsubscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:unsubscribe",
                data: {
                    to: [this.subscription],
                },
            });
        },
        fetch() {},
        // Without them, rows show every non-empty param
        fetchJobs() {
            axios
                .get("/api/jobs/")
                .then((response) => {
                    const jobs = new Map();
                    for (const job of response.data || []) {
                        jobs.set(job.name, {
                            defaults: Object.assign({}, ...(job.defaultParams || [])),
                            labels: job.labels || [],
                            interval: job.interval || "",
                        });
                    }
                    this.jobs = jobs;
                })
                .catch(() => {});
        },
        fetchNow(more = false) {
            this.fetch(more);
            this.fetch.flush();
        },
        applyUpdate(ev, fromFetch = false) {
            // A build started under a root that the feed shows goes in the
            // root's group (F2)
            const rootIndex = ev.root ? findInContainer(this.builds, "id", ev.root)[1] : undefined;
            if (rootIndex !== undefined) {
                const root = this.builds[rootIndex];
                const members = (root.members || []).filter((member) => member.id !== ev.id);
                members.push(ev);
                members.sort((a, b) => a.id - b.id);
                this.builds[rootIndex] = { ...root, members };
                return;
            }
            // A group from the server takes in the rows of its members,
            // shown on their own while their root wasn't loaded
            if (ev.members?.length) {
                const ids = new Set(ev.members.map((member) => member.id));
                this.builds = this.builds.filter((build) => !ids.has(build.id));
            }
            const index = findInContainer(this.builds, "id", ev.id)[1];
            if (index !== undefined) {
                const old = this.builds[index];
                // A live update of a root doesn't carry its group
                this.builds[index] = ev.members || !old.members ? ev : { ...ev, members: old.members, hiddenMembers: old.hiddenMembers };
            } else {
                if (!fromFetch) {
                    if (isFilteredUpdate(ev, this.filterObj)) {
                        this.filteredUpdates++;
                        return;
                    }
                }
                this.builds.push(ev);
                this.$forceUpdate();
            }
        },
        clearFilter() {
            if (!this.isFetching && !this.filterIsDirty) {
                this.filter = "";
                this.filteredUpdates = 0;
                this.fetchNow();
            }
        },
        onWSChange(value) {
            if (value) {
                this.subscribe();
            } else {
                this.unsubscribe();
            }
        },
        readSelected() {
            const id = Number(this.$route.query.build);
            this.selected = Number.isInteger(id) && id > 0 ? id : null;
        },
        onWideChange(event) {
            this.wide = event.matches;
        },
        // selectBuild shows a build beside the feed; the address names it,
        // so a reload or a shared link shows it again
        selectBuild(id) {
            this.$router.replace({ query: { ...this.$route.query, build: String(id) } });
        },
        closeBuild() {
            const query = { ...this.$route.query };
            delete query.build;
            this.$router.replace({ query });
        },
        // Up and down move to the next build, Esc closes, while a build
        // shows beside the feed and the user isn't typing or in a dialog
        onKeyDown(event) {
            if (!this.split || event.target.closest?.("input, textarea, select, [contenteditable], .cm-editor") || document.querySelector("dialog[open]")) {
                return;
            }
            if (event.key === "Escape") {
                this.closeBuild();
                return;
            }
            if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
                return;
            }
            const i = this.sortedBuilds.findIndex((build) => build.id === this.selected);
            const next = this.sortedBuilds[i + (event.key === "ArrowDown" ? 1 : -1)];
            if (next) {
                event.preventDefault();
                this.selectBuild(next.id);
                this.$nextTick(() => document.querySelector(`[data-cy-build="${next.id}"]`)?.scrollIntoView({ block: "nearest" }));
            }
        },
        // handleSetFilter searches for a term, e.g. branch=main, required
        // with +: alone, or added to the search with Ctrl/Cmd+click, since
        // plain terms mean "any of"
        handleSetFilter({ term, append }) {
            if (append && this.filter) {
                // Avoid adding duplicates by comparing unquoted values
                const clean = (t) => t.replace(/^[+-]/, "").replace(/^["']/, "").replace(/["']$/, "");
                const existingTerms = this.filter.split(/\s+/).map(clean);
                if (!existingTerms.includes(clean(term))) {
                    this.filter += " +" + term;
                }
            } else {
                this.filter = "+" + term;
            }
        },
    },
};
</script>

<style scoped>
/* The list beside the build, which takes the rest */
.feed-page.split {
    display: grid;
    grid-template-columns: minmax(16rem, 22rem) minmax(0, 1fr);
    gap: var(--space-4);
    align-items: start;
}

.feed-pane {
    position: sticky;
    inset-block-start: 64px;
    max-block-size: calc(100dvh - 80px);
    overflow-y: auto;
    padding-inline-start: var(--space-4);
    border-inline-start: 1px solid var(--outline-variant);
}

.feed-list {
    border-block-start: 1px solid var(--outline-variant);
}

.feed-empty {
    align-items: center;
    padding: var(--space-8) 0;
}
</style>
