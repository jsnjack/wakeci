<template>
  <div class="container grid-xl">
    <div class="input-group input-inline float-right py-1">
      <input
        class="form-input"
        type="text"
        :value="filter"
        title="Filter builds by ID, name, params and status"
        data-cy="filter"
        @input="evt=>filter=evt.target.value"
      >
      <button
        class="btn btn-action"
        :class="{'loading': isFetching}"
        @click.prevent="clearFilter"
      >
        <i
          class="icon"
          :class="filterIconType"
        />
      </button>
    </div>
    <table class="table table-striped">
      <thead>
        <th>#</th>
        <th>Name</th>
        <th class="hide-xs hide-sm">
          Params
        </th>
        <th class="hide-xs hide-sm hide-md">
          Tasks
        </th>
        <th>Status</th>
        <th class="hide-xs">
          Duration
        </th>
        <th>Actions</th>
      </thead>
      <tbody>
        <FeedItem
          v-for="item in sortedBuilds"
          :key="item.id"
          :build="item"
        />
      </tbody>
    </table>
    <div
      v-show="Object.keys(builds).length === 0"
      class="empty"
    >
      <p class="empty-title h5">
        Empty
      </p>
    </div>
    <button
      v-show="moreEnabled"
      class="btn btn-link float-right"
      :class="{'loading': isFetching}"
      @click.prevent="fetchNow(true)"
    >
      more...
    </button>
  </div>
</template>

<script>
import FeedItem from "@/components/FeedItem";
import axios from "axios";
import {findInContainer} from "@/store/utils";
import _ from "lodash";

const FetchItemsSize = 10;

export default {
    components: {FeedItem},
    data: function() {
        return {
            builds: [],
            subscription: "build:update:",
            isFetching: false, // request to the server is in progress
            filterIsDirty: false, // when user is still typing
            filter: "", // sent to the server, to filter builds out
            moreEnabled: true, // if makes sense to load more builds from the server
        };
    },
    computed: {
        sortedBuilds: function() {
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
        filterIconType: function() {
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
        filter: function() {
            this.filterIsDirty = true;
            // Reset builds if user starts to change filter
            this.builds = [];
            this.fetch();
        },
    },
    mounted() {
        document.title = "Feed - wakeci";
        this.fetchNow();
        this.subscribe();
        this.$on("new:log", this.applyNewLog);
    },
    destroyed() {
        this.unsubscribe();
    },
    created() {
        this.fetch = _.debounce((more = false) => {
            this.isFetching = true;
            let offset = 0;
            if (more) {
                offset = this.builds.length;
            }
            axios
                .get(`/api/feed/?offset=${offset}&filter=${this.filter}`)
                .then((response) => {
                    this.isFetching = false;
                    const data = response.data || [];
                    data.forEach((element) => {
                        this.applyUpdate(element);
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
            this.$eventHub.$on(this.subscription, this.applyUpdate);
        },
        unsubscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:unsubscribe",
                data: {
                    to: [this.subscription],
                },
            });
            this.$eventHub.$off(this.subscription);
        },
        fetch() {},
        fetchNow(more=false) {
            this.fetch(more);
            this.fetch.flush();
        },
        applyUpdate(ev) {
            const index = findInContainer(this.builds, "id", ev.id)[1];
            if (index !== undefined) {
                this.$set(
                    this.builds,
                    index,
                    Object.assign({}, this.builds[index], ev),
                );
            } else {
                this.builds.push(ev);
                this.$forceUpdate();
            }
        },
        clearFilter() {
            if (!this.isFetching && !this.filterIsDirty) {
                this.filter = "";
                this.fetchNow();
            }
        },
    },
};
</script>

<style scoped lang="scss">
</style>
