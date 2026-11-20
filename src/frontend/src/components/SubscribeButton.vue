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
import vuex from "vuex";

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
        ...vuex.mapState(["notifyOnBuildStatusUpdate"]),
        label() {
            return this.isSubscribed ? "Don't notify me" : "Notify me when it ends";
        },
        isSubscribed() {
            return !this.disabled && this.notifyOnBuildStatusUpdate.includes(this.buildId);
        },
    },
    methods: {
        toggleSubscription() {
            if (this.isSubscribed) {
                this.$store.commit("UNSUBSCRIBE_NOTIFICATION", this.buildId);
            } else {
                if (!("Notification" in window)) {
                    alert("This browser does not support system notifications");
                    return;
                }

                Notification.requestPermission().then((permission) => {
                    if (permission === "granted") {
                        this.$store.commit("SUBSCRIBE_NOTIFICATION", this.buildId);
                    }
                });
            }
        },
    },
};
</script>

<style scoped></style>
