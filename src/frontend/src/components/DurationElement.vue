<template>
    <span
        class="label tooltip tooltip-bottom c-hand"
        :data-tooltip="tooltipText"
        data-cy="duration"
        @click.prevent="toggleDurationMode"
        >{{ durationText }}</span
    >
</template>

<script>
import { runningDuration, doneDuration, updateDurationPeriod, toggleDurationMode } from "@/duration";
import vuex from "vuex";
import TimeAgo from "javascript-time-ago";

import en from "javascript-time-ago/locale/en.json";

TimeAgo.addDefaultLocale(en);
const ago = new TimeAgo("en-US");

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
    data: function () {
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
                case "timed out":
                    return true;
            }
            return false;
        },
        tooltipText() {
            return "Toggle between different time modes";
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
                switch (this.mode) {
                    case "duration":
                        this.durationText = runningDuration(this.item.startedAt);
                        return;
                    case "started at":
                        this.durationText = new Date(this.item.startedAt).toLocaleString();
                        return;
                    case "started":
                        this.durationText = ago.format(new Date(this.item.startedAt));
                        return;
                    default:
                        console.log(`Unknown durationMode ${this.mode}`);
                        this.toggleDurationMode();
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
                        this.durationText = ago.format(new Date(this.item.startedAt));
                        return;
                    default:
                        console.log(`Unknown durationMode ${this.mode}`);
                        this.toggleDurationMode();
                        return;
                }
            }
            return "";
        },
        onStatusChange() {
            if ((this.isDone && this.mode !== "started") || this.mode === "started at") {
                clearInterval(this.updateInterval);
            } else if ((this.item.status === "running" || this.mode === "started") && !this.updateInterval) {
                this.updateInterval = setInterval(
                    function () {
                        this.updateText();
                    }.bind(this),
                    updateDurationPeriod
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

<style
    scoped
    lang="scss"
></style>
