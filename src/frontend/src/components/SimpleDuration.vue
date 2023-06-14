<template>
    <div class="row">
        <i
            v-if="!hideIcon"
            class="small"
            >avg_time</i
        >
        <div>{{ durationText }}</div>
    </div>
</template>

<script>
import { runningDuration, doneDuration, updateDurationPeriod } from "@/duration";
import vuex from "vuex";

export default {
    props: {
        item: {
            required: true,
            type: Object,
        },
        hideIcon: {
            type: Boolean,
            default: false,
            required: false,
        },
    },
    data: function () {
        return {
            updateInterval: null,
            durationText: "",
        };
    },
    computed: {
        ...vuex.mapState(["durationMode"]),
        isDone() {
            switch (this.item.status) {
                case "failed":
                case "finished":
                case "aborted":
                case "skipped":
                case "timed out":
                    return true;
            }
            return false;
        },
    },
    watch: {
        "item.status": "onStatusChange",
        "item.duration": "onStatusChange",
        mode: "onStatusChange",
    },
    mounted() {
        this.onStatusChange();
    },
    beforeUnmount: function () {
        clearInterval(this.updateInterval);
    },
    methods: {
        updateText() {
            if (this.item.startedAt && this.item.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                this.durationText = "";
                return;
            }
            if (this.item.status === "running") {
                this.durationText = runningDuration(this.item.startedAt);
                return;
            }
            if (this.item.duration > 0) {
                this.durationText = doneDuration(this.item.duration);
                return;
            }
            return "";
        },
        onStatusChange() {
            if (this.isDone) {
                clearInterval(this.updateInterval);
            } else if (this.item.status === "running" && !this.updateInterval) {
                this.updateInterval = setInterval(
                    function () {
                        this.updateText();
                    }.bind(this),
                    updateDurationPeriod
                );
            }
            this.updateText();
        },
    },
};
</script>

<style scoped lang="scss"></style>
