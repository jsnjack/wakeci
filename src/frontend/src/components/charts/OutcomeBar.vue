<template>
    <span class="bar">
        <span
            class="stack"
            :style="{ width }"
        >
            <span
                v-for="part in parts"
                :key="part.key"
                :class="part.cls"
                :style="{ flexGrow: part.count }"
                :data-cy-outcome="part.key"
            ></span>
        </span>
    </span>
</template>

<script>
import { OUTCOMES } from "@/analytics";

// OutcomeBar shows a count of requests as a bar split by outcome, failures
// first like the stacks of StatusBarsChart, so they start at the same place
// in every row. A part with any requests stays visible, however few
export default {
    props: {
        // The counts by outcome, as RequestOutcomes of the API
        counts: {
            type: Object,
            required: true,
        },
        // The CSS width of the bar in its track, from barWidth
        width: {
            type: String,
            required: true,
        },
    },
    computed: {
        parts() {
            return OUTCOMES.filter((outcome) => this.counts[outcome.key] > 0).map((outcome) => ({
                key: outcome.key,
                cls: outcome.cls,
                count: this.counts[outcome.key],
            }));
        },
    },
};
</script>

<style scoped>
.bar {
    display: block;
    height: 0.6rem;
    border-radius: 0.2rem;
    background: var(--surface-container-highest);
    overflow: hidden;
}
.stack {
    display: flex;
    gap: 1px;
    height: 100%;
}
/* flex-basis 0 makes the parts as wide as their counts */
.stack > span {
    flex: 1 1 0;
    min-width: 2px;
}
</style>
