<template>
  <div
    class="tooltip tooltip-bottom"
    :data-tooltip="getProgressTooltip()"
  >
    <progress
      class="progress"
      :value="donePercent"
      max="100"
    />
  </div>
</template>

<script>
import {
    doneDurationSec,
} from "@/duration";

export default {
    props: {
        eta: {
            type: Number, // ns
            required: true,
        },
        startedAt: {
            type: String,
            required: true,
        },
        buildDuration: {
            type: Number,
            required: true,
        },
    },
    data: function() {
        return {
            donePercent: 0,
            updateInterval: null,
            etaInSec: 0,
        };
    },
    watch: {
        "startedAt": "onUpdate",
        "buildDuration": "onDone",
    },
    mounted() {
        this.etaInSec = Math.round(this.eta / 10**9);
        const numberOfDataPoints = 200;
        let refreshPeriod = this.etaInSec / numberOfDataPoints * 1000;
        if (refreshPeriod < 1000) {
            refreshPeriod = 1000;
        }
        if (!this.updateInterval) {
            this.updateInterval = setInterval(
                function() {
                    this.onUpdate();
                }.bind(this),
                refreshPeriod,
            );
        }
        this.onUpdate();
        this.onDone();
    },
    beforeUnmount: function() {
        clearInterval(this.updateInterval);
    },
    methods: {
        onUpdate() {
            if (this.startedAt && this.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                return;
            }
            const duration = (new Date().getTime() - new Date(this.startedAt).getTime()) / 1000;
            let p = duration / this.etaInSec * 100;
            if (p > 99) {
                p = 99;
                clearInterval(this.updateInterval);
            }
            this.donePercent = p;
        },
        onDone() {
            if (this.buildDuration) {
                this.donePercent = 100;
                clearInterval(this.updateInterval);
            }
        },
        getProgressTooltip() {
            if (this.startedAt && this.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                const eta = doneDurationSec(this.etaInSec);
                return `should take about ${eta}`;
            }
            if (this.donePercent !== 100) {
                const duration = (new Date().getTime() - new Date(this.startedAt).getTime()) / 1000;
                if (this.etaInSec > duration) {
                    const eta = doneDurationSec(this.etaInSec - duration);
                    return `eta ${eta}`;
                } else {
                    return "any moment";
                }
            }
            return "completed";
        },
    },
};
</script>

<style scoped lang="scss">
</style>
