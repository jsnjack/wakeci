<template>
    <div class="ui feed-page stack gap-3">
        <UiDialog
            v-model:open="isHelpDialogOpen"
            title="Search examples"
        >
            <ul class="search-examples stack gap-2">
                <li><code>status:failed</code> Failed builds</li>
                <li><code>status:failed "status:timed out"</code> Failed or timed out builds</li>
                <li><code>+status:failed -name:test_</code> Failed builds, excluding builds with names containing "test_"</li>
                <li><code>env=prod</code> Builds whose parameter "ENV" (or label "ENV_LABEL") is exactly "prod"</li>
                <li><code>+status:failed env=prod env=uat</code> Failed builds with "ENV" equal to "prod" or "uat"</li>
                <li><code>env:prod</code> "env:prod" anywhere, so also "env:production"</li>
            </ul>
            <p class="muted text-sm">A click on a value in a row searches for it; Ctrl+click adds it to the search.</p>
            <template #actions>
                <button
                    type="button"
                    class="btn"
                    @click="isHelpDialogOpen = false"
                >
                    Close
                </button>
            </template>
        </UiDialog>

        <div class="cluster nowrap gap-1">
            <label class="input-icon grow">
                <i class="icon">search</i>
                <input
                    type="search"
                    class="input"
                    data-cy="filter"
                    aria-label="Search builds"
                    placeholder="Search by id, job, status, branch=main…"
                    :value="filter"
                    @input="(evt) => (filter = evt.target.value)"
                />
                <span
                    v-show="isFetching"
                    class="feed-spinner status-ring"
                    aria-hidden="true"
                ></span>
            </label>
            <button
                type="button"
                class="btn btn-ghost btn-icon"
                aria-label="Clear the search"
                data-tip="Clear the search"
                :disabled="filter === ''"
                @click="clearFilter"
            >
                <i class="icon">backspace</i>
            </button>
            <button
                type="button"
                class="btn btn-ghost btn-icon tip-end"
                aria-label="Search examples"
                data-tip="Search examples"
                @click="isHelpDialogOpen = true"
            >
                <i class="icon">help</i>
            </button>
        </div>

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
            <FeedItem
                v-for="item in sortedBuilds"
                :key="item.id"
                :build="item"
                :job="jobs.get(item.name) || null"
                :now="now"
                @set-filter="handleSetFilter"
            />
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
</template>

<script>
import FeedItem from "@/components/FeedItem.vue";
import UiDialog from "@/components/ui/UiDialog.vue";
import { createFilterObj, findInContainer, isFilteredUpdate } from "@/store/utils.js";
import axios from "axios";
import debounce from "lodash/debounce";
import vuex from "vuex";

const FetchItemsSize = 10;

export default {
    components: { FeedItem, UiDialog },
    data: function () {
        return {
            builds: [],
            subscription: "build:update:",
            isFetching: false, // request to the server is in progress
            isHelpDialogOpen: false,
            filterIsDirty: false, // when user is still typing
            filter: "", // sent to the server, to filter builds out
            moreEnabled: true, // if makes sense to load more builds from the server
            filteredUpdates: 0, // When `filter` is active, updates which do not much are counted here
            filterObj: null,
            // Each job's { defaults: { NAME: value }, labels: [NAME] }, so
            // a row can show the labels first and leave defaults out
            jobs: new Map(),
            // The time the rows show durations against, moved on every second
            now: Date.now(),
        };
    },
    computed: {
        ...vuex.mapState(["ws"]),
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
    },
    unmounted() {
        this.unsubscribe();
        this.emitter.off(this.subscription, this.applyUpdate);
        clearInterval(this.clock);
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
                .get(`/api/feed?${before}q=${encodeURIComponent(this.filter)}`)
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
                        jobs.set(job.name, { defaults: Object.assign({}, ...(job.defaultParams || [])), labels: job.labels || [] });
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
            const index = findInContainer(this.builds, "id", ev.id)[1];
            if (index !== undefined) {
                this.builds[index] = ev;
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
.feed-list {
    border-block-start: 1px solid var(--outline-variant);
}

.feed-spinner {
    position: absolute;
    inset-inline-end: 10px;
    inline-size: 16px;
    block-size: 16px;
    --_c: var(--primary);
}

.feed-page input[type="search"] {
    padding-inline-end: 34px;
}

.feed-empty {
    align-items: center;
    padding: var(--space-8) 0;
}

.search-examples {
    padding-inline-start: 0;
    list-style: none;
}

.search-examples code {
    margin-inline-end: var(--space-2);
    padding: 1px 6px;
    border-radius: 4px;
    background-color: var(--surface-container-highest);
}
</style>
