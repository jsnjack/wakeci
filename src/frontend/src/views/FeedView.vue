<template>
  <div class="container">
    <table class="table table-striped">
      <thead>
        <th>#</th>
        <th>Name</th>
        <th class="hide-xs hide-sm">Params</th>
        <th class="hide-xs hide-sm">Tasks</th>
        <th>Status</th>
        <th class="hide-xs">Duration</th>
        <th>Actions</th>
      </thead>
      <tbody>
        <FeedItem v-for="item in sortedBuilds" :key="item.id" :build="item"></FeedItem>
      </tbody>
    </table>
    <div class="empty" v-show="Object.keys(builds).length === 0">
      <p class="empty-title h5">Empty</p>
    </div>
    <ul class="pagination float-right">
      <li class="page-item" v-bind:class="{ disabled: isFirstPage }">
        <a href="#" @click.prevent="fetchPrevious">Previous</a>
      </li>
      <li>|</li>
      <li class="page-item" v-bind:class="{ disabled: isLastPage }">
        <a href="#" @click.prevent="fetchNext">Next</a>
      </li>
    </ul>
  </div>
</template>

<script>
import FeedItem from "@/components/FeedItem";
import {APIURL} from "@/store/communication";
import axios from "axios";
import {findInContainer} from "@/store/utils";


export default {
    components: {FeedItem},
    mounted() {
        this.fetch();
        this.subscribe();
        this.$on("new:log", this.applyNewLog);
    },
    destroyed() {
        this.unsubscribe();
    },
    watch: {
        "$route.query.page": "onQueryChange",
    },
    computed: {
        sortedBuilds: function() {
            return [...this.builds].sort((a, b) => a.id < b.id);
        },
        isFirstPage: function() {
            return this.page === 1;
        },
        isLastPage: function() {
            return this.builds.length === 0;
        },
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
        fetch() {
            axios
                .get(APIURL + "/feed/?page=" + this.page)
                .then((response) => {
                    this.builds = response.data || [];
                })
                .catch((error) => {});
        },
        applyUpdate(ev) {
            const index = findInContainer(this.builds, "id", ev.id)[1];
            if (index !== undefined) {
                this.$set(
                    this.builds,
                    index,
                    Object.assign({}, this.builds[index], ev)
                );
            } else {
                // Only push new items to the first page
                if (this.page === 1) {
                    this.builds.push(ev);
                }
            }
        },
        fetchNext() {
            this.$router.push({path: "/", query: {page: this.page + 1}});
        },
        fetchPrevious() {
            this.$router.push({path: "/", query: {page: this.page - 1}});
        },
        onQueryChange(val) {
            this.page = val;
            this.fetch();
        },
    },
    data: function() {
        return {
            builds: [],
            subscription: "build:update:",
            page: this.$route.query.page || 1,
        };
    },
};
</script>

<style scoped lang="scss">
</style>
