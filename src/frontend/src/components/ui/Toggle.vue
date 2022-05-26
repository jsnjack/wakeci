<template>
    <div :class="['switch', { on: value }, { off: !value }]" @click="$emit('input', !value)">
        <span class="switch-toggle"/>
    </div>
</template>

<script>
export default {
    props: {
        value: {
            type: Boolean,
            required: true,
        },
    },
};
</script>

<style lang="scss">
.switch {
    @apply h-3 w-5 ring-1 ring-gray-border transition-colors rounded-lg relative duration-200;
    & > .switch-toggle {
        @apply h-3 w-3 transition-all absolute rounded-full top-1/2 -translate-y-1/2 duration-300;
    }
    &.on {
        @apply bg-primary-light;
        & > .switch-toggle {
            @apply bg-primary left-0;
        }
    }
    &.off {
        @apply bg-gray-light;
        & > .switch-toggle {
            @apply bg-primary translate-x-2;
        }
    }
}
</style>
