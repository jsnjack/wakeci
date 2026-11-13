<template>
    <svg
        class="spark"
        :width="width"
        :height="height"
        aria-hidden="true"
    >
        <path :d="path" />
    </svg>
</template>

<script>
// SparkLine shows how a measure went over the days, with no axis; the tile
// it sits in names the measure and its value
export default {
    props: {
        // Numbers, null for days without a value
        values: {
            type: Array,
            required: true,
        },
        width: {
            type: Number,
            default: 120,
        },
        height: {
            type: Number,
            default: 28,
        },
    },
    computed: {
        path() {
            const known = this.values.filter((v) => v !== null && v !== undefined);
            if (known.length < 2) {
                return "";
            }
            const lo = Math.min(...known);
            const span = Math.max(...known) - lo || 1;
            const step = (this.width - 4) / (this.values.length - 1);
            let d = "";
            let pen = false;
            this.values.forEach((v, i) => {
                if (v === null || v === undefined) {
                    pen = false;
                    return;
                }
                const x = 2 + i * step;
                const y = this.height - 3 - ((v - lo) / span) * (this.height - 6);
                d += `${pen ? "L" : "M"}${x.toFixed(1)} ${y.toFixed(1)}`;
                pen = true;
            });
            return d;
        },
    },
};
</script>

<style scoped>
.spark {
    display: block;
    margin-top: 0.25rem;
}
path {
    fill: none;
    stroke: var(--chart-1);
    stroke-width: 1.5;
    stroke-linejoin: round;
}
</style>
