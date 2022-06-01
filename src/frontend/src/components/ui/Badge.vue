<template>
  <span :class="['badge', type]">
      {{ text }}
  </span>
</template>

<script>
export default {
    name: 'Badge',
    props: {
        text: {
            type: String,
            required: true,
        },
        type: {
            type: String,
            required: false,
            default: 'success',
            validate: val => ['success', 'danger', 'warning'].includes(val),
        },
    },
}
</script>

<style lang="scss" scoped>
.badge {
    @apply py-0.5 px-1 text-xs text-white capitalize rounded-sm;
    &.success {
        @apply bg-success;
    }
    &.danger {
        @apply bg-danger;
    }
    &.warning {
        @apply bg-warning;
    }
}
</style>