<template>
    <div
        class="tooltip tooltip-bottom"
        :data-tooltip="getProgressTooltip()"
    >
        <progress
            class="progress"
            :value="done"
            :max="total"
        />
    </div>
</template>

<script>
export default {
    props: {
        done: {
            type: Number,
            required: true,
        },
        total: {
            type: Number,
            required: true,
        },
    },
    methods: {
        getProgressTooltip() {
            if (this.done === this.total) {
                return "completed";
            }
            return `${this.done} of ${this.total} tasks`;
        },
    },
};
</script>

<style
    scoped
    lang="scss"
></style>
