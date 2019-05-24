<template>
<div>
    <table class="table table-striped">
        <thead>
            <th>Name</th>
            <th>Build #</th>
            <th>Tasks</th>
            <th>Status</th>
        </thead>
        <tbody>
            <FeedUpdateComp v-for="item in feed"
                :key="item.id"
                :feed="item">
            </FeedUpdateComp>
        </tbody>
    </table>
    <div class="empty" v-show="feed.length === 0">
    <div class="empty-icon">
        <i class="icon icon-arrow-right"></i>
    </div>
    <p class="empty-title h5">Empty</p>
    </div>
</div>
</template>

<script>
import vuex from "vuex";
import FeedUpdateComp from "@/components/FeedUpdateComp.vue";

export default {
    components: {FeedUpdateComp},
    computed: {
        ...vuex.mapState([
            "feed",
        ]),
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
</style>
