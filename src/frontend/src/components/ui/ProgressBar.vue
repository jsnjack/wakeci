<template>
  <span class="progress-bar">
    <span :class="['progress', type]" :style="{width: progress + '%'}"/>
  </span>
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
            validate: val => ['success', 'warning', 'danger'].includes(val),
        },
    },
}
</script>

<style lang="scss" scoped>
.progress-bar {
    @apply rounded h-1 bg-gray-light w-full;
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
    }
}
</style>