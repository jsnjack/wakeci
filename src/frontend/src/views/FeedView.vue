<template>
    <nav class="no-space">
        <div class="max field label prefix border left-round">
            <i>search</i>
            <input
                type="text"
                data-cy="filter"
                :value="filter"
                @input="(evt) => (filter = evt.target.value)"
            />
            <a :class="{ loader: isFetching }"></a>
            <label>Filter builds by ID, name, params and status</label>
        </div>
        <button
            class="large right-round secondary"
            @click.prevent="clearFilter"
        >
            <i>backspace</i>
        </button>
    </nav>

    <article
        class="fill"
        v-show="filteredUpdates !== 0"
        data-cy="filtered-updates"
    >
        <p>{{ filteredUpdates }} updates have been filtered beacuse of the active filter</p>
    </article>

    <div data-cy="feed-container">
        <FeedItem
            v-for="item in sortedBuilds"
            :key="item.id"
            :build="item"
            :params-index="paramsIndex"
        />
    </div>

    <div
        v-if="sortedBuilds.length === 0 && !isFetching && !filterIsDirty"
        class="fill medium-height middle-align center-align"
        data-cy="no-builds-found"
    >
        <div class="center-align">
            <i class="extra">water</i>
            <h5>No builds found</h5>
        </div>
    </div>

    <nav class="no-space">
        <div class="max"></div>
        <button
            v-show="moreEnabled && !isFetching && !filterIsDirty"
            @click.prevent="fetchNow(true)"
        >
            more...
        </button>
    </nav>
</template>

<script>
import FeedItem from "@/components/FeedItem.vue";
import vuex from "vuex";
import axios from "axios";
import { findInContainer, isFilteredUpdate } from "@/store/utils.js";
import _ from "lodash";

const FetchItemsSize = 10;

export default {
    components: { FeedItem },
    data: function () {
        return {
            builds: [],
            subscription: "build:update:",
            isFetching: false, // request to the server is in progress
            filterIsDirty: false, // when user is still typing
            filter: "", // sent to the server, to filter builds out
            moreEnabled: true, // if makes sense to load more builds from the server
            paramsIndex: 0, // Params index to display on the feed page
            filteredUpdates: 0, // When `filter` is active, updates which do not much are counted here
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
        filter: function () {
            this.filterIsDirty = true;
            // Reset builds if user starts to change filter
            this.builds = [];
            this.fetch();
        },
        "ws.connected": "onWSChange",
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Feed");

        // Restore filter from URL in address bar
        const url = new URL(window.location.href);
        if (url.searchParams.has("filter")) {
            const filterValue = url.searchParams.get("filter");
            this.filter = decodeURIComponent(filterValue);
        }

        this.fetchNow();
        this.subscribe();
        this.emitter.on(this.subscription, this.applyUpdate);
    },
    unmounted() {
        this.unsubscribe();
        this.emitter.off(this.subscription, this.applyUpdate);
    },
    created() {
        this.fetch = _.debounce((more = false) => {
            this.isFetching = true;
            let offset = 0;
            if (more) {
                offset = this.builds.length;
            }
            axios
                .get(`/api/feed?offset=${offset}&filter=${encodeURIComponent(this.filter)}`)
                .then((response) => {
                    this.isFetching = false;

                    // Put filter value in address bar to allow copying and link sharing
                    const url = new URL(window.location.href);
                    if (this.filter === "") {
                        url.searchParams.delete("filter");
                    } else {
                        const newSearch = url.searchParams;
                        newSearch.set("filter", encodeURIComponent(this.filter));
                        url.search = newSearch.toString();
                    }
                    this.$router.replace(url.pathname + url.search);

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
                .catch((error) => {});
            this.filterIsDirty = false;
        }, 500);
    },
    methods: {
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
                    if (isFilteredUpdate(ev, this.filter)) {
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
    },
};
</script>

<style scoped lang="scss"></style>
