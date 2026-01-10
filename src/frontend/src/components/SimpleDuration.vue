<template>
    <div class="row">
        <i
            v-if="!minimalisticMode"
            class="small"
            >avg_time</i
        >
        <div :data-startedAt="item.startedAt">{{ durationText }}</div>
        <div
            v-if="durationTooltip"
            class="tooltip bottom"
        >
            {{ durationTooltip }}
        </div>
    </div>
</template>

<script>
import { doneDuration, doneDurationSec, runningDuration } from "@/duration";

export default {
    props: {
        item: {
            required: true,
            type: Object,
        },
        minimalisticMode: {
            type: Boolean,
            default: false,
            required: false,
        },
    },
    data: function () {
        return {
            updateInterval: null,
            durationText: "",
            durationTooltip: "",
            updatePeriod: 1000,
        };
    },
    computed: {
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
        getMainTasks() {
            return this.item.tasks.filter((el) => {
                return el.kind === "main";
            });
        },
        getDoneTasks() {
            return this.getMainTasks.filter((el) => {
                return el.status !== "pending" && el.status !== "running";
            }).length;
        },
        getTotalTasks() {
            return this.getMainTasks.length;
        },
    },
    watch: {
        "item.status": "onStatusChange",
        "item.duration": "onStatusChange",
        mode: "onStatusChange",
    },
    mounted() {
        if (this.item.eta) {
            const etaInSec = Math.round(this.item.eta / 10 ** 9);
            const numberOfDataPoints = 200;
            // Default update period is once a second. Make it depends on the
            // eta of the task
            let period = (etaInSec / numberOfDataPoints) * 1000;
            if (period > this.updatePeriod) {
                this.updatePeriod = period;
            }
        }
        this.onStatusChange();
    },
    beforeUnmount: function () {
        clearInterval(this.updateInterval);
    },
    methods: {
        updateText() {
            // Build is in the queue
            if (this.item.startedAt && this.item.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                this.durationText = "";
                return;
            }

            // Build is in progress
            if (this.item.status === "running") {
                this.durationText = runningDuration(this.item.startedAt);
                if (!this.minimalisticMode) {
                    // Add information about the progress status
                    if (this.item.eta) {
                        const duration = (new Date().getTime() - new Date(this.item.startedAt).getTime()) / 1000;
                        const p = Math.round((duration / Math.round(this.item.eta / 10 ** 9)) * 100);
                        // Can be infinity in case of indefinite tasks
                        if (Number.isInteger(p)) {
                            this.durationText += ` (${p}%)`;
                            if (p > 100) {
                                this.durationTooltip = "any moment";
                            } else {
                                const eta = doneDurationSec(Math.round(this.item.eta / 10 ** 9 - duration));
                                this.durationTooltip = `eta ${eta}`;
                            }
                        }
                    } else {
                        this.durationText += ` (${this.getDoneTasks}/${this.getTotalTasks})`;
                    }
                }
                return;
            }
            // Build is done, show the result
            if (this.item.duration > 0) {
                this.durationText = doneDuration(this.item.duration);
                if (this.item.eta) {
                    const p = Math.round((this.item.duration / Math.round(this.item.eta)) * 100);
                    if (Number.isInteger(p)) {
                        if (p > 100) {
                            this.durationTooltip = p - 100 + "% slower than usual";
                        } else if (p < 100) {
                            this.durationTooltip = 100 - p + "% faster than usual";
                        }
                    }
                }
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
                    this.updatePeriod
                );
            }
            this.updateText();
        },
    },
};
</script>

<style scoped >
div {
    cursor: default;
}
</style>
