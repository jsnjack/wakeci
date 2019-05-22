<template>
<div>
    <table>
        <thead>
            <td>Name</td>
            <td>Build #</td>
            <td>Tasks</td>
            <td>Status</td>
        </thead>
        <FeedUpdateComp v-for="item in feed"
            :key="item.id"
            :feed="item">
        </FeedUpdateComp>
    </table>
    <p v-show="feed.length === 0">Empty</p>
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
