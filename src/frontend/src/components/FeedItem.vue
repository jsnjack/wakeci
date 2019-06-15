<template>
    <tr>
        <td>
            {{ build.name }}
        </td>
        <td>
            <router-link :to="{ name: 'build', params: { count: build.count, job_name: build.name }}">{{ build.count }}</router-link>
        </td>
        <td class="tooltip tooltip-right" :data-tooltip="getProgressTooltip">
            <progress
                class="progress"
                :value="build.done_tasks"
                :max="build.total_tasks">
            </progress>
        </td>
        <td>
            <BuildStatus :status="build.status"></BuildStatus>
        </td>
    </tr>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";

export default {
    components: {BuildStatus},
    props: {
        build: {
            type: Object,
            required: true,
        },
    },
    methods: {
    },
    computed: {
        getProgressTooltip() {
            return `${this.build.done_tasks} of ${this.build.total_tasks}`;
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">

</style>
