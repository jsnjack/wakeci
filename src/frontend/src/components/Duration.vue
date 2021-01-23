<template>
  <span
    class="label tooltip tooltip-bottom"
    :data-tooltip="tooltipText"
  >{{ durationText }}</span>
</template>

<script>
import {
    runningDuration,
    doneDuration,
    updateDurationPeriod,
} from "@/duration";
import vuex from "vuex";
import {format} from "timeago.js";


export default {
    props: {
        item: {
            required: true,
            type: Object,
        },
    },
    data: function() {
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
                return true;
            }
            return false;
        },
        tooltipText() {
            const d = new Date(this.item.startedAt).toLocaleString();
            return `Started at: ${d}`;
        },
    },
    watch: {
        "item.status": "onStatusChange",
        "item.duration": "onStatusChange",
        "durationMode": "onStatusChange",
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
                switch (this.durationMode) {
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
                switch (this.durationMode) {
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
            console.log(this.durationMode);
            if (this.isDone || this.durationMode === "started at") {
                clearInterval(this.updateInterval);
            } else if (this.item.status === "running" && !this.updateInterval) {
                this.updateInterval = setInterval(
                    function() {
                        this.updateText();
                    }.bind(this),
                    updateDurationPeriod,
                );
            }
            this.updateText();
        },
    },
};
</script>

<style scoped lang="scss">
span:hover {
  cursor: default;
}
</style>
