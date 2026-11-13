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

// StatusLegend names the colours of the build statuses, finished first
export default {
    computed: {
        statuses() {
            return [...STATUSES].reverse();
        },
    },
};
</script>
