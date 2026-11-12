<template>
    <div
        class="chart"
        data-cy="task-bars"
    >
        <svg
            v-if="width"
            :width="width"
            :height="height"
            role="img"
            :aria-label="`Duration of the last ${builds.length} builds by task`"
        >
            <HatchPattern :id="hatchID" />
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
            <text
                :x="left - 6"
                :y="markerY + 9"
                text-anchor="end"
            >
                result
            </text>
            <g
                v-for="bar in bars"
                :key="bar.id"
                class="mark link"
                data-cy="task-bar"
                @click="open(bar.id)"
            >
                <title>{{ bar.title }}</title>
                <rect
                    class="hit"
                    :x="bar.x"
                    :y="top"
                    :width="step"
                    :height="height - top - 16"
                />
                <rect
                    v-for="segment in bar.segments"
                    :key="segment.key"
                    :class="segment.cls"
                    :x="bar.x + 1"
                    :y="segment.y"
                    :width="barWidth"
                    :height="segment.height"
                    rx="1.5"
                />
                <rect
                    :class="bar.statusCls"
                    :style="bar.statusStyle"
                    :x="bar.x + 1"
                    :y="markerY + 12 - bar.markerHeight"
                    :width="barWidth"
                    :height="bar.markerHeight"
                    rx="1"
                />
            </g>
            <text
                v-if="builds.length"
                :x="left"
                :y="height - 2"
            >
                #{{ builds[0].id }}
            </text>
            <text
                v-if="builds.length > 1"
                :x="width"
                :y="height - 2"
                text-anchor="end"
            >
                #{{ builds[builds.length - 1].id }}
            </text>
        </svg>
    </div>
</template>

<script>
import { axisDuration, formatDuration, niceDurationMax, statusOf, uniqueID } from "@/analytics";
import chartWidth from "./chartWidth";
import HatchPattern from "./HatchPattern.vue";

// TaskBarsChart shows each build as a bar of its duration, stacked by task
// in the order the tasks ran, with the build's result under it. A bar opens
// its build
export default {
    components: { HatchPattern },
    mixins: [chartWidth],
    props: {
        // InsightBuild of the API, oldest first
        builds: {
            type: Array,
            required: true,
        },
        // Series of a task name: series-1 to series-5 for the main tasks
        // with their own colour, series-other for the rest; hooks are
        // series-hooks
        seriesOf: {
            type: Function,
            required: true,
        },
        height: {
            type: Number,
            default: 210,
        },
    },
    data() {
        return { hatchID: uniqueID("task-hatch"), left: 42, top: 6 };
    },
    computed: {
        // The bars end above the result markers and the build numbers
        inner() {
            return this.height - this.top - 40;
        },
        markerY() {
            return this.top + this.inner + 6;
        },
        step() {
            return (this.width - this.left) / Math.max(1, this.builds.length);
        },
        barWidth() {
            return Math.max(1, this.step - 2);
        },
        max() {
            return niceDurationMax(Math.max(1, ...this.builds.map((b) => this.total(b))));
        },
        ticks() {
            return [0, 0.5, 1].map((f) => ({ y: this.top + this.inner - f * this.inner, text: axisDuration(this.max * f) }));
        },
        bars() {
            return this.builds.map((b, i) => {
                let y = this.top + this.inner;
                const segments = [];
                b.tasks.forEach((task, t) => {
                    if (!task.duration) {
                        return;
                    }
                    const height = (task.duration / this.max) * this.inner;
                    y -= height;
                    segments.push({
                        key: t,
                        cls: task.kind === "main" ? this.seriesOf(task.name) : "series-hooks",
                        y: y + (height > 3 ? 1 : 0),
                        height: Math.max(0.5, height - (height > 3 ? 1 : 0)),
                    });
                });
                const status = statusOf(b.status);
                const lines = [`#${b.id} ${status.label}${b.failedTask ? ` at ${b.failedTask}` : ""}`];
                for (const task of b.tasks) {
                    if (task.status !== "skipped") {
                        lines.push(`${task.name}${task.kind === "main" ? "" : ` (${task.kind})`}: ${formatDuration(task.duration)}`);
                    }
                }
                lines.push(`Total ${formatDuration(b.duration)}`);
                return {
                    id: b.id,
                    x: this.left + i * this.step,
                    segments,
                    statusCls: status.cls,
                    statusStyle: b.status === "failed" ? { fill: `url(#${this.hatchID})` } : null,
                    markerHeight: b.status === "finished" ? 5 : b.status === "aborted" ? 8 : 12,
                    title: lines.join("\n"),
                };
            });
        },
    },
    methods: {
        total(build) {
            return build.tasks.reduce((sum, task) => sum + task.duration, 0);
        },
        open(id) {
            this.$router.push({ name: "build", params: { id } });
        },
    },
};
</script>
