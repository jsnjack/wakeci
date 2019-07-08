<template>
  <div class="container">
    <table class="table table-striped">
      <thead>
        <th>Name</th>
        <th>Build #</th>
        <th>Params</th>
        <th>Tasks</th>
        <th>Status</th>
        <th>Actions</th>
      </thead>
      <tbody>
        <FeedItem v-for="item in sortedBuilds" :key="item.id" :build="item"></FeedItem>
      </tbody>
    </table>
    <div class="empty" v-show="Object.keys(builds).length === 0">
      <p class="empty-title h5">Empty</p>
    </div>
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
                "type": "in:subscribe",
                "data": {
                    "to": [this.subscription],
                },
            });
            this.$eventHub.$on(this.subscription, this.applyUpdate);
        },
        unsubscribe() {
            this.$store.commit("WS_SEND", {
                "type": "in:unsubscribe",
                "data": {
                    "to": [this.subscription],
                },
            });
            this.$eventHub.$off(this.subscription);
        },
        fetch() {
            axios.get(APIURL + "/feed/")
                .then((response) => {
                    this.builds = response.data || [];
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
        applyUpdate(ev) {
            const index = findInContainer(this.builds, "id", ev.id)[1];
            if (index !== undefined) {
                this.$set(this.builds, index, Object.assign({}, this.builds[index], ev));
            } else {
                this.builds.push(ev);
            }
        },
    },
    data: function() {
        return {
            builds: [],
            subscription: "build:update:",
        };
    },
};
</script>

<style scoped lang="scss">
</style>
