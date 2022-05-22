<template>
    <div class="container grid-xl">
        <div class="input-group input-inline float-right py-1">
            <input
                class="form-input"
                type="text"
                :value="filter"
                title="Filter builds by ID, name, params and status"
                data-cy="filter"
                @input="(evt) => (filter = evt.target.value)"
            />
            <div class="dropdown dropdown-right text-left">
                <div class="btn-group">
                    <button
                        class="btn btn-action"
                        :class="{ loading: isFetching }"
                        @click.prevent="clearFilter"
                    >
                        <i
                            class="icon"
                            :class="filterIconType"
                        />
                    </button>
                    <a
                        class="btn dropdown-toggle hide-xs hide-sm"
                        tabindex="0"
                    >
                        <i class="icon icon-caret" />
                    </a>
                    <ul class="menu hide-xs hide-sm">
                        <li class="menu-item">
                            <a
                                href="#"
                                @click.prevent="toggleAdvancedSyntaxModal"
                                >View search syntax</a
                            >
                        </li>
                    </ul>
                </div>
            </div>
        </div>
        <div class="clearfix" />
        <span
            v-show="filteredUpdates !== 0"
            class="label label-warning"
            data-cy="filtered-updates"
            >{{ filteredUpdates }} updates have been filtered</span
        >
        <table class="table table-striped">
            <thead>
                <th>#</th>
                <th>Name</th>
                <th class="hide-xs hide-sm">
                    <span
                        class="badge c-hand"
                        :data-badge="paramsIndex || ''"
                        data-cy="params-index-button"
                        title="Toggle between different parameters"
                        @click.prevent="toggleParams(false)"
                    >
                        Params
                    </span>
                    <i
                        v-show="paramsIndex"
                        class="icon icon-cross c-hand"
                        data-cy="params-index-button-clean"
                        @click.prevent="toggleParams(true)"
                    />
                </th>
                <th class="hide-xs hide-sm hide-md">Tasks</th>
                <th>Status</th>
                <th class="hide-xs">
                    <span
                        class="text-capitalize badge c-hand"
                        title="Toggle between different time modes"
                        @click.prevent="toggleDurationMode()"
                    >
                        {{ durationMode }}
                    </span>
                </th>
                <th>Actions</th>
            </thead>
            <tbody data-cy="feed-tbody">
                <FeedItem
                    v-for="item in sortedBuilds"
                    :key="item.id"
                    :build="item"
                    :params-index="paramsIndex"
                />
            </tbody>
        </table>
        <div
            v-show="Object.keys(builds).length === 0"
            class="empty"
        >
            <p class="empty-title h5">Empty</p>
        </div>
        <button
            v-show="moreEnabled"
            class="btn btn-link float-right"
            :class="{ loading: isFetching }"
            @click.prevent="fetchNow(true)"
        >
            more...
        </button>
    </div>
    <div
        class="modal"
        :class="{ active: showAdvancedSyntaxModal }"
    >
        <a
            href="#"
            class="modal-overlay"
            aria-label="Close"
            @click.prevent="toggleAdvancedSyntaxModal"
        ></a>
        <div class="modal-container">
            <div class="modal-header">
                <a
                    href="#"
                    class="btn btn-clear float-right"
                    aria-label="Close"
                    @click.prevent="toggleAdvancedSyntaxModal"
                ></a>
                <div class="modal-title h5">Search syntax</div>
            </div>
            <div class="modal-body">
                <div class="content text-left">
                    <ul>
                        <li>
                            Returns only builds which <span class="text-italic">ID</span>, <span class="text-italic">name</span>,
                            <span class="text-italic">params</span> or <span class="text-italic">status</span> contains <span class="text-bold">any</span> of
                            the space-separated words
                        </li>
                        <li>Requires presence of the prefixed with <code>+</code> words</li>
                        <li>Requires absence of the prefixed with <code>-</code> words</li>
                        <li>Phrases can be wrapped in single or double quotes</li>
                    </ul>
                    <span> Example: <code>aborted "timed out" -yesterday +'cpu info'</code></span>
                </div>
            </div>
        </div>
    </div>
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
            showAdvancedSyntaxModal: false,
        };
    },
    computed: {
        ...vuex.mapState(["ws", "durationMode"]),
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
                return "";
            }
            if (this.filterIsDirty) {
                return "icon-more-horiz";
            }
            if (this.filter === "") {
                return "icon-search";
            }
            return "icon-cross";
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
        document.title = "Feed - wakeci";
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
            this.$store.commit("TOGGLE_DURATION_MODE");
        },
        toggleAdvancedSyntaxModal() {
            this.showAdvancedSyntaxModal = !this.showAdvancedSyntaxModal;
        },
    },
};
</script>

<style
    scoped
    lang="scss"
></style>
