<template>
    <div
        class="chart"
        data-cy="line-chart"
    >
        <svg
            v-if="width"
            :width="width"
            :height="height"
            role="img"
            :aria-label="label"
        >
            <g class="grid">
                <g
                    v-for="tick in ticks"
                    :key="tick.y"
                >
                    <line
                        :x1="left"
                        :x2="width"
                        :y1="tick.y"
                        :y2="tick.y"
                    />
                    <text
                        :x="left - 6"
                        :y="tick.y + 4"
                        text-anchor="end"
                    >
                        {{ tick.text }}
                    </text>
                </g>
            </g>
            <path
                v-for="(d, i) in bandPaths"
                :key="'band' + i"
                class="band"
                :d="d"
            />
            <path
                v-if="upperPath"
                class="upper"
                :d="upperPath"
            />
            <path
                class="line"
                :d="linePath"
            />
            <circle
                v-if="lastPoint"
                class="dot"
                :cx="lastPoint.x"
                :cy="lastPoint.y"
                r="3.5"
            />
            <g
                v-for="(point, i) in points"
                :key="point.label + i"
                class="mark"
            >
                <title>{{ point.title }}</title>
                <rect
                    class="hit"
                    :x="x(i) - step / 2"
                    :y="top"
                    :width="step"
                    :height="inner"
                />
            </g>
            <text
                v-for="axisLabel in labels"
                :key="axisLabel.text"
                :x="axisLabel.x"
                :y="height - 4"
                :text-anchor="axisLabel.anchor"
            >
                {{ axisLabel.text }}
            </text>
        </svg>
    </div>
</template>

<script>
import chartWidth from "./chartWidth";

// LineChart draws one measure over days: a line of value, and, if the
// points have upper, a band up to it, such as the median and the 95th
// percentile. A point without a value leaves a gap
export default {
    mixins: [chartWidth],
    props: {
        // { label, value, upper, title }; value null for no data
        points: {
            type: Array,
            required: true,
        },
        min: {
            type: Number,
            default: 0,
        },
        max: {
            type: Number,
            required: true,
        },
        // Writes an axis value
        format: {
            type: Function,
            required: true,
        },
        // Names the chart for screen readers
        label: {
            type: String,
            required: true,
        },
        height: {
            type: Number,
            default: 160,
        },
    },
    data() {
        return { left: 42, top: 8, bottom: 20 };
    },
    computed: {
        inner() {
            return this.height - this.top - this.bottom;
        },
        step() {
            return (this.width - this.left - 4) / Math.max(1, this.points.length - 1);
        },
        ticks() {
            return [0, 0.5, 1].map((f) => {
                const value = this.min + (this.max - this.min) * f;
                return { y: this.y(value), text: this.format(value) };
            });
        },
        linePath() {
            return this.path("value");
        },
        upperPath() {
            return this.points.some((p) => p.upper !== undefined && p.upper !== null) ? this.path("upper") : "";
        },
        bandPaths() {
            if (!this.upperPath) {
                return [];
            }
            const paths = [];
            let run = [];
            const flush = () => {
                if (run.length > 1) {
                    const top = run.map((i) => `${this.x(i)} ${this.y(this.points[i].upper)}`);
                    const bottom = [...run].reverse().map((i) => `${this.x(i)} ${this.y(this.points[i].value)}`);
                    paths.push(`M${top.join("L")}L${bottom.join("L")}Z`);
                }
                run = [];
            };
            this.points.forEach((p, i) => {
                if (this.has(p.value) && this.has(p.upper)) {
                    run.push(i);
                } else {
                    flush();
                }
            });
            flush();
            return paths;
        },
        lastPoint() {
            for (let i = this.points.length - 1; i >= 0; i--) {
                if (this.has(this.points[i].value)) {
                    return { x: this.x(i), y: this.y(this.points[i].value) };
                }
            }
            return null;
        },
        labels() {
            const n = this.points.length;
            const count = Math.max(2, Math.min(n, Math.floor(this.width / 100)));
            const labels = [];
            for (let k = 0; k < count; k++) {
                const i = Math.round((k * (n - 1)) / Math.max(1, count - 1));
                if (labels.some((l) => l.i === i)) {
                    continue;
                }
                labels.push({ i, text: this.points[i].label, x: this.x(i), anchor: i === 0 ? "start" : i === n - 1 ? "end" : "middle" });
            }
            return labels;
        },
    },
    methods: {
        has(value) {
            return value !== null && value !== undefined;
        },
        x(i) {
            return this.left + i * this.step;
        },
        y(value) {
            const clamped = Math.min(this.max, Math.max(this.min, value));
            return this.top + this.inner - ((clamped - this.min) / (this.max - this.min || 1)) * this.inner;
        },
        path(key) {
            let d = "";
            let pen = false;
            this.points.forEach((p, i) => {
                if (!this.has(p[key])) {
                    pen = false;
                    return;
                }
                d += `${pen ? "L" : "M"}${this.x(i).toFixed(1)} ${this.y(p[key]).toFixed(1)}`;
                pen = true;
            });
            return d;
        },
    },
};
</script>

<style scoped>
.line {
    fill: none;
    stroke: var(--chart-1);
    stroke-width: 2;
    stroke-linejoin: round;
    stroke-linecap: round;
}
.upper {
    fill: none;
    stroke: var(--chart-1);
    stroke-width: 1;
    stroke-dasharray: 3 3;
}
.band {
    fill: color-mix(in srgb, var(--chart-1) 22%, transparent);
}
.dot {
    fill: var(--chart-1);
    stroke: var(--surface-container);
    stroke-width: 2;
}
</style>
