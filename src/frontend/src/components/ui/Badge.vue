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
            validate: (val) => ['success', 'danger', 'warning', 'info', 'blank'].includes(val),
        },
    },
};
</script>

<style lang="scss" scoped>
.badge {
    @apply py-0.5 px-1 text-xs ring-1 capitalize rounded-sm no-underline;
    &.success {
        @apply bg-success-light text-success-dark ring-success-dark;
    }
    &.danger {
        @apply bg-danger-light text-danger-dark ring-danger;
    }
    &.warning {
        @apply bg-warning-light text-warning-dark ring-warning;
    }
    &.info {
        @apply bg-info-light text-info-dark ring-info;
    }
	&.blank {
		@apply bg-white text-secondary ring-secondary;
	}
}
</style>
