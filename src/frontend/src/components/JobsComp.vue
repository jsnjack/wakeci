<template>
<div>
    <table>
        <thead>
            <td>Name</td>
            <td>Total executions</td>
            <td>Actions</td>
        </thead>
        <JobComp v-for="item in jobs"
            :key="item.name"
            :job="item">
        </JobComp>
    </table>
    <p v-show="jobs.length === 0">Empty</p>
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
