<template>
  <div class="container">
    <table class="table table-striped">
      <thead>
        <th>Name</th>
        <th>Build #</th>
        <th>Tasks</th>
        <th>Status</th>
      </thead>
      <tbody>
        <FeedItem v-for="item in builds" :key="item.id" :build="item"></FeedItem>
      </tbody>
    </table>
    <div class="empty" v-show="builds.length === 0">
      <p class="empty-title h5">Empty</p>
    </div>
  </div>
</template>

<script>
import FeedItem from "@/components/FeedItem.vue";
import {APIURL} from "@/store/communication";
import axios from "axios";

export default {
    components: {FeedItem},
    mounted() {
        this.fetch();
        this.subscribe();
    },
    destroyed() {
        this.unsubscribe();
    },
    methods: {
        subscribe() {
            this.$store.commit("ACTIVE_SUBSCRIPTION", this.subscription);
            this.ws.obj.sendMessage({
                "type": "in:subscribe",
                "data": {
                    "to": this.subscription,
                    "id": this.id,
                },
            });
        },
        unsubscribe() {
            this.$store.commit("ACTIVE_SUBSCRIPTION", "");
            this.ws.obj.sendMessage({
                "type": "in:unsubscribe",
                "data": {
                    "to": this.subscription,
                },
            });
        },
        fetch() {
            axios.get(APIURL + "/feed/")
                .then(function(response) {
                    console.log(response);
                })
                .catch(function(error) {
                    console.log(error);
                });
        },
    },
    data: function() {
        return {
            builds: [{
                name: "build_project",
                count: 10,
                done_tasks: 3,
                total_tasks: 5,
                status: "running",
            }],
        };
    },
};
</script>

<style scoped lang="scss">
</style>
