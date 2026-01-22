<template>
    <button
        class="circle transparent"
        :disabled="disabled"
        @click.prevent="toggleSubscription"
    >
        <i v-if="!isSubscribed">notifications_none</i>
        <i v-else>notifications_active</i>
        <div class="tooltip bottom" v-if="!disabled">
            {{ isSubscribed ? "Unsubscribe from notifications" : "Subscribe to notifications" }}
        </div>
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
