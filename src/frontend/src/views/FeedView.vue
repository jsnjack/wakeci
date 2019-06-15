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

export default {
    components: {FeedItem},
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
