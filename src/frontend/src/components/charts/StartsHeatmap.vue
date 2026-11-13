<template>
    <div
        class="chart"
        data-cy="starts-heatmap"
    >
        <svg
            v-if="width"
            :width="width"
            :height="height"
            role="img"
            aria-label="Builds started by weekday and hour"
        >
            <g
                v-for="(row, r) in starts"
                :key="r"
            >
                <text
                    :x="left - 6"
                    :y="top + r * cellHeight + cellHeight / 2 + 4"
                    text-anchor="end"
                >
                    {{ weekdays[r] }}
                </text>
                <rect
                    v-for="(count, h) in row"
                    :key="h"
                    :x="left + h * cellWidth + 1"
                    :y="top + r * cellHeight + 1"
                    :width="Math.max(1, cellWidth - 2)"
                    :height="cellHeight - 2"
                    rx="2"
                    :style="{ fill: color(count) }"
                >
                    <title>{{ weekdays[r] }} {{ hour(h) }}–{{ hour(h + 1) }}: {{ count }} {{ count === 1 ? "build" : "builds" }}</title>
                </rect>
            </g>
            <text
                v-for="h in [0, 6, 12, 18]"
                :key="h"
                :x="left + h * cellWidth"
                :y="height - 3"
            >
                {{ hour(h) }}
            </text>
        </svg>
        <div class="chart-legend">
            <span>fewer</span>
            <span
                v-for="level in [0, 0.25, 0.5, 0.75, 1]"
                :key="level"
                class="swatch"
                :style="{ backgroundColor: color(level * max) }"
            ></span>
            <span>more</span>
        </div>
    </div>
</template>

<script>
import chartWidth from "./chartWidth";

// StartsHeatmap shows when builds start: a cell per weekday and hour, the
// darker the more builds
export default {
    mixins: [chartWidth],
    props: {
        // [weekday Monday first][hour] counts
        starts: {
            type: Array,
            required: true,
        },
    },
    data() {
        return { left: 36, top: 2, bottom: 18, weekdays: ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"] };
    },
    computed: {
        cellWidth() {
            return (this.width - this.left) / 24;
        },
        cellHeight() {
            return Math.max(14, Math.min(24, this.cellWidth * 0.9));
        },
        height() {
            return this.top + 7 * this.cellHeight + this.bottom;
        },
        max() {
            return Math.max(1, ...this.starts.flat());
        },
    },
    methods: {
        // One hue, from the surface to full, by the square root so that
        // quiet hours still show
        color(count) {
            if (!count) {
                return "var(--surface-container-highest)";
            }
            const p = Math.round(15 + 85 * Math.sqrt(count / this.max));
            return `color-mix(in srgb, var(--chart-1) ${p}%, var(--surface-container-highest))`;
        },
        hour(h) {
            return `${String(h % 24).padStart(2, "0")}:00`;
        },
    },
};
</script>
