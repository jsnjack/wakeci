<template>
    <tr>
        <td>
            {{ feed.name }}
        </td>
        <td>
            {{ feed.count }}
        </td>
        <td class="tooltip tooltip-right" :data-tooltip="getProgressTooltip">
            <progress
                class="progress"
                :value="feed.done_tasks"
                :max="feed.total_tasks">
            </progress>
        </td>
        <td>
            <span class="label label-rounded" :class="getStatusClass">{{ feed.status }}</span>
        </td>
    </tr>
</template>

<script>
// import vuex from "vuex";

export default {
    props: {
        feed: {
            type: Object,
            required: true,
        },
    },
    methods: {
    },
    computed: {
        getProgressTooltip() {
            return `${this.feed.done_tasks} of ${this.feed.total_tasks}`;
        },
        getStatusClass() {
            switch (this.feed.status) {
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
