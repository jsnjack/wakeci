<template>
    <tr>
        <td>
            {{ build.name }}
        </td>
        <td>
            <router-link :to="{ name: 'build', params: { count: build.count, job_name: build.name }}">{{ build.count }}</router-link>
            <!-- <router-link to="/build" class="btn btn-link">Feed</router-link> -->
            <!-- {{ build.count }} -->
        </td>
        <td class="tooltip tooltip-right" :data-tooltip="getProgressTooltip">
            <progress
                class="progress"
                :value="build.done_tasks"
                :max="build.total_tasks">
            </progress>
        </td>
        <td>
            <span class="label label-rounded" :class="getStatusClass">{{ build.status }}</span>
        </td>
    </tr>
</template>

<script>

export default {
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
        getStatusClass() {
            switch (this.build.status) {
            case "running":
                return "label-warning";
            case "failed":
                return "label-error";
            case "finished":
                return "label-success";
            }
            // pending
            return "";
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">

</style>
