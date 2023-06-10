<template>
    <div class="small-space">
        <div
            class="progress left"
            :value="done"
            :max="total"
        ></div>
        <div class="tooltip bottom">{{ getProgressTooltip }}</div>
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

<style scoped lang="scss"></style>
