<template>
  <progress
    class="progress"
    :value="donePercent"
    max="100"
  />
</template>

<script>
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
    },
    beforeDestroy: function() {
        clearInterval(this.updateInterval);
    },
    methods: {
        onUpdate() {
            if (this.startedAt && this.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                return;
            }
            const duration = (new Date().getTime() - new Date(this.startedAt).getTime()) / 1000;
            if (duration <= this.etaInSec) {
                this.donePercent = duration / this.etaInSec * 100;
            } else {
                this.donePercent = 100;
                clearInterval(this.updateInterval);
            }
        },
    },
};
</script>

<style scoped lang="scss">
</style>
