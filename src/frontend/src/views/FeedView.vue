<template>
    <div>
        <div class="input-group input-inline float-right py-1">
            <input
                class="form-input"
                type="text"
                :value="filter"
                title="Filter builds by ID, name, params and status"
                data-cy="filter"
                placeholder="Filter..."
                @input="(evt) => (filter = evt.target.value)"
            />
            <button
                class="btn btn-action"
                :class="{ loading: isFetching }"
                @click.prevent="clearFilter"
            >
                <i class="icon" :class="filterIconType" />
            </button>
        </div>

        <div class="feed-items">
            <FeedItem v-for="build in builds" :build="build" :key="build.id" />
        </div>

        <button
            v-show="moreEnabled"
            class="btn btn-primary load-btn"
            :class="{ loading: isFetching }"
            @click.prevent="fetchNow(true)"
        >
            Load more...
        </button>
    </div>
</template>

<script>
import FeedItem from '@/components/FeedItem.vue';
import vuex from 'vuex';
import axios from 'axios';
import { findInContainer, isFilteredUpdate } from '@/store/utils.js';
import _ from 'lodash';

const FetchItemsSize = 10;

export default {
    components: {
        FeedItem,
    },
    data: function () {
        return {
            builds: [],
            subscription: 'build:update:',
            isFetching: false, // request to the server is in progress
            filterIsDirty: false, // when user is still typing
            filter: '', // sent to the server, to filter builds out
            moreEnabled: true, // if makes sense to load more builds from the server
            paramsIndex: 0, // Params index to display on the feed page
            filteredUpdates: 0, // When `filter` is active, updates which do not much are counted here
            showAdvancedSyntaxModal: false,
        };
    },
    computed: {
        ...vuex.mapState(['ws', 'durationMode']),
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
        filterIconType: function () {
            if (this.isFetching) {
                return '';
            }
            if (this.filterIsDirty) {
                return 'icon-more-horiz';
            }
            if (this.filter === '') {
                return 'icon-search';
            }
            return 'icon-cross';
        },
    },
    watch: {
        filter: function () {
            this.filterIsDirty = true;
            // Reset builds if user starts to change filter
            this.builds = [];
            this.fetch();
        },
        'ws.connected': 'onWSChange',
    },
    mounted() {
        document.title = 'Feed - wakeci';

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
<<<<<<< HEAD
                    if (this.filter === "") {
                        url.searchParams.delete("filter");
=======
                    if (this.filter === '') {
                        url.hash = '';
>>>>>>> 2820fe0 (Add partial FeedItem new card)
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
            this.$store.commit('WS_SEND', {
                type: 'in:subscribe',
                data: {
                    to: [this.subscription],
                },
            });
        },
        unsubscribe() {
            this.$store.commit('WS_SEND', {
                type: 'in:unsubscribe',
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
            const index = findInContainer(this.builds, 'id', ev.id)[1];
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
                this.filter = '';
                this.filteredUpdates = 0;
                this.fetchNow();
            }
        },
        toggleParams(reset = false) {
            if (reset) {
                this.paramsIndex = 0;
            } else {
                this.paramsIndex++;
            }
        },
        onWSChange(value) {
            if (value) {
                this.subscribe();
            } else {
                this.unsubscribe();
            }
        },
        toggleDurationMode() {
            this.$store.commit('TOGGLE_DURATION_MODE');
        },
        toggleAdvancedSyntaxModal() {
            this.showAdvancedSyntaxModal = !this.showAdvancedSyntaxModal;
        },
    },
};
</script>

<style scoped lang="scss">
.load-btn {
    @apply float-right mt-4;
}
.feed-items {
    @apply flex flex-col gap-4 w-full;
}
</style>
