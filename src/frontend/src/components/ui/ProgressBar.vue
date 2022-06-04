<template>
    <div class="progress-bar">
        <span
            role="progressbar"
            max="100"
            :class="['progress', type]"
            :style="{ width: progress + '%' }"
        />
    </div>
</template>

<script>
export default {
    name: 'ProgressBar',
    props: {
        progress: {
            type: Number,
            required: true,
        },
        type: {
            type: String,
            required: false,
            default: 'succes',
            validate: (val) => ['success', 'warning', 'danger', 'info'].includes(val),
        },
    },
};
</script>

<style lang="scss" scoped>
.progress-bar {
    @apply rounded h-1 bg-gray-light w-full flex;
    .progress {
        @apply rounded h-1;
        &.success {
            @apply bg-success;
        }
        &.warning {
            @apply bg-warning;
        }
        &.danger {
            @apply bg-danger;
        }
        &.info {
            @apply bg-info animate-pulse;
        }
    }
}
</style>
