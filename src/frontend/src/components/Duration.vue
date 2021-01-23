<template>
  <span
    class="label tooltip tooltip-bottom c-hand"
    :data-tooltip="tooltipText"
    @click.prevent="toggleDurationMode"
  >{{ durationText }}</span>
</template>

<script>
import {
    runningDuration,
    doneDuration,
    updateDurationPeriod,
    toggleDurationMode,
} from "@/duration";
import vuex from "vuex";
import {format} from "timeago.js";


export default {
    props: {
        item: {
            required: true,
            type: Object,
        },
        useGlobalDurationModeState: {
            required: false,
            type: Boolean,
            default: false,
        },
    },
    data: function() {
        return {
            updateInterval: null,
            durationText: "",
            localMode: "duration",
        };
    },
    computed: {
        ...vuex.mapState(["durationMode"]),
        isDone() {
            switch (this.item.status) {
            case "failed":
            case "finished":
            case "aborted":
                return true;
            }
            return false;
        },
        tooltipText() {
            return "Click to toggle between different time modes";
        },
        mode() {
            if (this.useGlobalDurationModeState) {
                return this.durationMode;
            }
            return this.localMode;
        },
    },
    watch: {
        "item.status": "onStatusChange",
        "item.duration": "onStatusChange",
        "mode": "onStatusChange",
    },
    mounted() {
        this.onStatusChange();
    },
    beforeDestroy: function() {
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
                switch (this.mode) {
                case "duration":
                    this.durationText = runningDuration(this.item.startedAt);
                    return;
                case "started at":
                    this.durationText = new Date(this.item.startedAt).toLocaleString();
                    return;
                case "started":
                    this.durationText = format(new Date(this.item.startedAt));
                    return;
                }
            }
            if (this.item.duration > 0) {
                switch (this.mode) {
                case "duration":
                    this.durationText = doneDuration(this.item.duration);
                    return;
                case "started at":
                    this.durationText = new Date(this.item.startedAt).toLocaleString();
                    return;
                case "started":
                    this.durationText = format(new Date(this.item.startedAt));
                    return;
                }
            }
            return "";
        },
        onStatusChange() {
            console.log("STATUS change", this.mode, this.item.status);
            if ((this.isDone && this.mode !== "started") || this.mode === "started at") {
                console.log("Clear interval");
                clearInterval(this.updateInterval);
            } else if ((this.item.status === "running" || this.mode === "started") && !this.updateInterval) {
                console.log("Start interval");
                this.updateInterval = setInterval(
                    function() {
                        this.updateText();
                    }.bind(this),
                    updateDurationPeriod,
                );
            }
            this.updateText();
        },
        toggleDurationMode() {
            if (this.useGlobalDurationModeState) {
                this.$store.commit("TOGGLE_DURATION_MODE");
            } else {
                this.localMode = toggleDurationMode(this.localMode);
            }
        },
    },
};
</script>

<style scoped lang="scss">
</style>
