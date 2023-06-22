<template>
    <div class="row">
        <i
            v-if="!minimalisticMode"
            class="small"
            >avg_time</i
        >
        <div>{{ durationText }}</div>
    </div>
</template>

<script>
import { runningDuration, doneDuration, updateDurationPeriod } from "@/duration";

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
                if (!this.minimalisticMode) {
                    // Add information about the progress status
                    if (this.item.eta) {
                        const duration = (new Date().getTime() - new Date(this.item.startedAt).getTime()) / 1000;
                        const p = Math.round((duration / Math.round(this.item.eta / 10 ** 9)) * 100);
                        this.durationText += ` (${p}%)`;
                    } else {
                        this.durationText += ` (${this.getDoneTasks}/${this.getTotalTasks})`;
                    }
                }
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
