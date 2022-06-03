<template>
    <div
        role="switch"
        :aria-label="`Dark mode ${modelValue ? 'on' : 'off'}`"
        :class="['switch', { on: modelValue }, { off: !modelValue }]"
        @click="$emit('update:modelValue', !modelValue)"
    >
        <span class="switch-toggle" />
    </div>
</template>

<script>
export default {
    props: {
        modelValue: {
            type: Boolean,
            required: true,
        },
    },
};
</script>

<style lang="scss">
.switch {
    @apply ring-1 ring-gray-border transition-colors rounded-full relative duration-200 z-10;
    height: 18px;
    width: 30px;
    & > .switch-toggle {
        @apply h-4 w-4 transition-all absolute rounded-full top-1/2 -translate-y-1/2 duration-500 z-20;
    }
    &.on {
        @apply bg-primary-light;
        & > .switch-toggle {
            @apply bg-primary;
            right: 1px;
        }
    }
    &.off {
        @apply bg-gray-light;
        & > .switch-toggle {
            @apply bg-primary;
            --tw-translate-x: 1px;
        }
    }
}
</style>
