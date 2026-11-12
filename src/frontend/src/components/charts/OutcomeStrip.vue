<template>
    <svg
        class="strip"
        :width="width"
        :height="height"
        role="img"
        :aria-label="summary"
        data-cy="outcome-strip"
    >
        <HatchPattern :id="hatchID" />
        <g
            v-for="(bar, i) in bars"
            :key="bar.id"
            class="mark link"
            @click="open(bar.id)"
        >
            <title>{{ bar.title }}</title>
            <rect
                class="hit"
                :x="(offset + i) * step"
                y="0"
                :width="step"
                :height="height"
            />
            <rect
                :class="bar.cls"
                :style="bar.style"
                :x="(offset + i) * step"
                :y="height - bar.height"
                :width="step - 2"
                :height="bar.height"
                rx="1.5"
                :data-cy-status="bar.status"
            />
        </g>
    </svg>
</template>

<script>
import { dateTime, formatDuration, statusOf, uniqueID } from "@/analytics";
import HatchPattern from "./HatchPattern.vue";

// OutcomeStrip shows a job's last builds, newest on the right. Height as
// well as colour tells the result: a short bar finished, a full one failed
// or timed out, a middle one was aborted. A bar opens its build
export default {
    components: { HatchPattern },
    props: {
        // BuildOutcome of the API, oldest first
        builds: {
            type: Array,
            required: true,
        },
        slots: {
            type: Number,
            default: 20,
        },
    },
    data() {
        return { hatchID: uniqueID("strip-hatch"), step: 7, height: 18 };
    },
    computed: {
        width() {
            return this.slots * this.step;
        },
        offset() {
            return Math.max(0, this.slots - this.builds.length);
        },
        bars() {
            return this.builds.slice(-this.slots).map((b) => {
                const status = statusOf(b.status);
                const when = dateTime(b.startedAt);
                const lines = [`#${b.id} ${status.label}${b.failedTask ? ` at ${b.failedTask}` : ""}`];
                if (when) {
                    lines.push(`${when}, ${formatDuration(b.duration)}`);
                }
                return {
                    id: b.id,
                    status: b.status,
                    cls: status.cls,
                    style: b.status === "failed" ? { fill: `url(#${this.hatchID})` } : null,
                    height: b.status === "finished" ? 7 : b.status === "aborted" ? 11 : this.height,
                    title: lines.join("\n"),
                };
            });
        },
        summary() {
            const failed = this.builds.filter((b) => b.status === "failed" || b.status === "timed out").length;
            return `Last ${this.builds.length} builds, ${failed} failed or timed out`;
        },
    },
    methods: {
        open(id) {
            this.$router.push({ name: "build", params: { id } });
        },
    },
};
</script>

<style scoped>
.strip {
    display: block;
}
.hit {
    fill: transparent;
}
.mark:hover .hit {
    fill: color-mix(in srgb, var(--on-surface) 10%, transparent);
}
.link {
    cursor: pointer;
}
</style>
