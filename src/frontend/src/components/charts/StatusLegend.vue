<template>
    <div class="chart-legend">
        <span
            v-for="status in statuses"
            :key="status.key"
        >
            <span
                class="swatch"
                :class="status.cls"
            ></span>
            <i>{{ status.icon }}</i> {{ status.label }}
        </span>
    </div>
</template>

<script>
import { STATUSES } from "@/analytics";

// StatusLegend names the colours of the build statuses, finished first, or
// of other series that StatusBarsChart stacks
export default {
    props: {
        series: {
            type: Array,
            default: () => STATUSES,
        },
    },
    computed: {
        statuses() {
            return [...this.series].reverse();
        },
    },
};
</script>
