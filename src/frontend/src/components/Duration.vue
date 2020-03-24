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
    },
    mounted() {
        this.onStatusChange();
    },
    beforeDestroy: function() {
        clearInterval(this.updateInterval);
    },
    methods: {
        updateDuration() {
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
                    function() {
                        this.updateDuration();
                    }.bind(this),
                    updateDurationPeriod,
                );
            }
            this.updateDuration();
        },
    },
};
</script>

<style scoped lang="scss">
span:hover {
  cursor: default;
}
</style>
