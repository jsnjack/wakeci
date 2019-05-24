<template>
<div>
    <table class="table table-striped">
        <thead>
            <th>Name</th>
            <th>Total executions</th>
            <th>Actions</th>
        </thead>
        <tbody>
            <JobComp v-for="item in jobs"
                :key="item.name"
                :job="item">
            </JobComp>
        </tbody>
    </table>

    <div class="empty" v-show="jobs.length === 0">
    <div class="empty-icon">
        <i class="icon icon-arrow-right"></i>
    </div>
    <p class="empty-title h5">Empty</p>
    </div>

    <div>
        <button class="btn btn-success">Create</button>
    </div>
</div>
</template>

<script>
import vuex from "vuex";
import JobComp from "@/components/JobComp";

export default {
    components: {JobComp},
    computed: {
        ...vuex.mapState([
            "jobs",
        ]),
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">

</style>
