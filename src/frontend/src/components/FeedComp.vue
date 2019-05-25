<template>
<div>
    <table class="table table-striped">
        <thead>
            <th>Job name</th>
            <th>Build #</th>
            <th>Tasks</th>
            <th>Status</th>
        </thead>
        <tbody>
            <BuildUpdateComp v-for="item in builds"
                :key="item.id"
                :build="item">
            </BuildUpdateComp>
        </tbody>
    </table>
    <div class="empty" v-show="builds.length === 0">
    <div class="empty-icon">
        <i class="icon icon-arrow-right"></i>
    </div>
    <p class="empty-title h5">Empty</p>
    </div>
</div>
</template>

<script>
import vuex from "vuex";
import BuildUpdateComp from "@/components/BuildUpdateComp.vue";

export default {
    components: {BuildUpdateComp},
    computed: {
        ...vuex.mapState([
            "builds",
        ]),
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
</style>
