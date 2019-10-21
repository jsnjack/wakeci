<template>
  <div class="container grid-xl">
    <table class="table table-striped">
      <thead>
        <th>#</th>
        <th>Name</th>
        <th class="hide-xs hide-sm">Params</th>
        <th class="hide-xs hide-sm hide-md">Tasks</th>
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
    <button @click.prevent="fetch(true)" class="btn btn-link float-right" :class="{'loading': isFetching}">more...</button>
  </div>
</template>

<script>
import FeedItem from "@/components/FeedItem";
import axios from "axios";
import {findInContainer} from "@/store/utils";

export default {
    components: {FeedItem},
    mounted() {
        document.title = "Feed - wakeci";
        this.fetch();
        this.subscribe();
        this.$on("new:log", this.applyNewLog);
    },
    destroyed() {
        this.unsubscribe();
    },
    computed: {
        sortedBuilds: function() {
            return [...this.builds].sort((a, b) => a.id < b.id);
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
        fetch(more=false) {
            let offset = 0;
            if (more) {
                offset = this.builds.length;
            }
            this.isFetching = true;
            axios
                .get("/api/feed/?offset=" + offset)
                .then((response) => {
                    this.isFetching = false;
                    (response.data || []).forEach((element) => {
                        this.applyUpdate(element);
                    });
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
                this.builds.push(ev);
            }
        },
    },
    data: function() {
        return {
            builds: [],
            subscription: "build:update:",
            isFetching: false,
        };
    },
};
</script>

<style scoped lang="scss">
</style>
