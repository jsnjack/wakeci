<template>
    <div
        class="chart"
        data-cy="status-bars"
    >
        <svg
            v-if="width"
            :width="width"
            :height="height"
            role="img"
            :aria-label="summary"
        >
            <HatchPattern :id="hatchID" />
            <g class="grid">
                <g
                    v-for="tick in ticks"
                    :key="tick.value"
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
                        {{ tick.value }}
                    </text>
                </g>
            </g>
            <g
                v-for="bar in bars"
                :key="bar.day"
                class="mark"
                data-cy="status-bar"
            >
                <title>{{ bar.title }}</title>
                <rect
                    class="hit"
                    :x="bar.x"
                    :y="top"
                    :width="step"
                    :height="inner"
                />
                <rect
                    v-for="segment in bar.segments"
                    :key="segment.key"
                    :class="segment.cls"
                    :style="segment.style"
                    :x="bar.x + gap / 2"
                    :y="segment.y"
                    :width="barWidth"
                    :height="segment.height"
                    rx="1.5"
                />
            </g>
            <text
                v-for="label in labels"
                :key="label.text"
                :x="label.x"
                :y="height - 4"
                text-anchor="middle"
            >
                {{ label.text }}
            </text>
        </svg>
    </div>
</template>

<script>
import { STATUSES, dayLabel, niceMax, uniqueID } from "@/analytics";
import chartWidth from "./chartWidth";
import HatchPattern from "./HatchPattern.vue";

// StatusBarsChart shows the builds of each day as a bar, stacked by status
// with failures at the baseline
export default {
    components: { HatchPattern },
    mixins: [chartWidth],
    props: {
        // AnalyticsDay of the API, oldest first
        days: {
            type: Array,
            required: true,
        },
        height: {
            type: Number,
            default: 180,
        },
    },
    data() {
        return { hatchID: uniqueID("hatch"), left: 32, top: 6, bottom: 20 };
    },
    computed: {
        inner() {
            return this.height - this.top - this.bottom;
        },
        step() {
            return (this.width - this.left) / Math.max(1, this.days.length);
        },
        gap() {
            return this.step > 6 ? 2 : 0.5;
        },
        barWidth() {
            return Math.max(1, this.step - this.gap);
        },
        max() {
            return niceMax(Math.max(0, ...this.days.map((day) => this.total(day))));
        },
        ticks() {
            return [0, 0.5, 1].map((f) => ({ value: Math.round(this.max * f), y: this.top + this.inner - f * this.inner }));
        },
        bars() {
            return this.days.map((day, i) => {
                let y = this.top + this.inner;
                const segments = [];
                for (const status of STATUSES) {
                    const count = day[status.key];
                    if (!count) {
                        continue;
                    }
                    const height = (count / this.max) * this.inner;
                    y -= height;
                    segments.push({
                        key: status.key,
                        cls: status.cls,
                        style: status.key === "failed" ? { fill: `url(#${this.hatchID})` } : null,
                        y: y + (height > 3 ? 1 : 0),
                        height: Math.max(1, height - (height > 3 ? 1 : 0)),
                    });
                }
                const lines = [...STATUSES]
                    .reverse()
                    .filter((s) => day[s.key])
                    .map((s) => `${day[s.key]} ${s.label}`);
                return {
                    day: day.day,
                    x: this.left + i * this.step,
                    segments,
                    title: `${dayLabel(day.day, true)}: ${this.total(day)} builds${lines.length ? "\n" + lines.join("\n") : ""}`,
                };
            });
        },
        labels() {
            const every = Math.max(1, Math.ceil(this.days.length / Math.max(2, Math.floor(this.width / 90))));
            const labels = [];
            for (let i = 0; i < this.days.length; i += every) {
                labels.push({ text: dayLabel(this.days[i].day), x: this.left + (i + 0.5) * this.step });
            }
            return labels;
        },
        summary() {
            const total = this.days.reduce((sum, day) => sum + this.total(day), 0);
            return `${total} builds in ${this.days.length} days`;
        },
    },
    methods: {
        total(day) {
            return STATUSES.reduce((sum, status) => sum + (day[status.key] || 0), 0);
        },
    },
};
</script>
