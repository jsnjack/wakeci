<template>
    <button
        class="circle transparent"
        @click.prevent="toggleSubscription"
    >
        <i v-if="!isSubscribed">notifications_none</i>
        <i v-else>notifications_active</i>
        <div class="tooltip bottom">
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
    },
    computed: {
        ...vuex.mapState(["notifications"]),
        isSubscribed() {
            return this.notifications.includes(this.buildId);
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
