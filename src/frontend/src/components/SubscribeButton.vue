<template>
    <button
        type="button"
        data-cy="subscribe-button"
        class="btn btn-ghost btn-icon"
        :disabled="disabled"
        :aria-label="label"
        :aria-pressed="isSubscribed ? 'true' : 'false'"
        :data-tip="disabled ? null : label"
        @click="toggleSubscription"
    >
        <i class="icon">{{ isSubscribed ? "notifications_active" : "notifications_none" }}</i>
    </button>
</template>

<script>
import { askNotificationPermission } from "@/store/utils.js";

// SubscribeButton asks to be told when a build ends, on any page and after
// a reload (L5 in docs/ui-roadmap.md)
export default {
    props: {
        buildId: {
            type: Number,
            required: true,
        },
        disabled: {
            type: Boolean,
            required: false,
            default: false,
        },
    },
    computed: {
        label() {
            return this.isSubscribed ? "Don't notify me" : "Notify me when it ends";
        },
        isSubscribed() {
            return !this.disabled && this.$store.state.watches.builds.includes(this.buildId);
        },
    },
    methods: {
        toggleSubscription() {
            if (this.isSubscribed) {
                this.$store.commit("UNWATCH_BUILD", this.buildId);
                return;
            }
            this.$store.commit("WATCH_BUILD", this.buildId);
            askNotificationPermission();
        },
    },
};
</script>

<style scoped></style>
