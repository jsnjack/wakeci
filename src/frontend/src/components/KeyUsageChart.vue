<template>
    <span
        class="usage"
        data-cy="key-usage"
    >
        <svg
            :width="width"
            :height="height"
            :viewBox="`0 0 ${width} ${height}`"
            role="img"
            :aria-label="summary"
        >
            <rect
                v-for="(bar, i) in bars"
                :key="bar.day"
                :x="i * step"
                :y="height - bar.height"
                :width="barWidth"
                :height="bar.height"
                :class="bar.uses > 0 ? 'bar' : 'empty'"
                data-cy="key-usage-bar"
            >
                <title>{{ bar.label }}: {{ requests(bar.uses) }}</title>
            </rect>
        </svg>
        <span data-cy="key-usage-total">{{ total.toLocaleString() }} in {{ days }} days</span>
    </span>
</template>

<script>
// KeyUsageChart shows an API key's requests per day, today last, as tiny
// bars; hovering a bar names its day and count. Days are the server's, so
// the chart ends on the day the server says is today.
export default {
    props: {
        // The days with requests: [{ day: "2026-11-12", uses: 3 }]
        usesByDay: {
            type: Array,
            default: () => [],
        },
        // The server's date
        today: {
            type: String,
            required: true,
        },
    },
    data: function () {
        return { days: 30, barWidth: 3, step: 4, height: 18 };
    },
    computed: {
        width() {
            return this.days * this.step - (this.step - this.barWidth);
        },
        bars() {
            const uses = new Map(this.usesByDay.map((d) => [d.day, d.uses]));
            const end = new Date(this.today + "T00:00:00Z");
            const days = [];
            for (let ago = this.days - 1; ago >= 0; ago--) {
                const date = new Date(end);
                date.setUTCDate(end.getUTCDate() - ago);
                const day = date.toISOString().slice(0, 10);
                days.push({
                    day,
                    uses: uses.get(day) || 0,
                    label: date.toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" }),
                });
            }
            const max = Math.max(1, ...days.map((d) => d.uses));
            // A day without requests is a thin line, so the days stay visible
            return days.map((d) => ({ ...d, height: d.uses > 0 ? Math.max(2, Math.round((d.uses / max) * this.height)) : 1 }));
        },
        total() {
            return this.bars.reduce((sum, bar) => sum + bar.uses, 0);
        },
        summary() {
            if (this.total === 0) {
                return `No requests in the last ${this.days} days`;
            }
            const busiest = this.bars.reduce((a, b) => (b.uses > a.uses ? b : a));
            return `${this.requests(this.total)} in the last ${this.days} days, most on ${busiest.label} (${busiest.uses})`;
        },
    },
    methods: {
        requests(n) {
            return `${n.toLocaleString()} ${n === 1 ? "request" : "requests"}`;
        },
    },
};
</script>

<style scoped>
.usage {
    display: inline-flex;
    align-items: flex-end;
    gap: 0.5rem;
    vertical-align: bottom;
}
.bar {
    fill: var(--primary);
}
.empty {
    fill: var(--outline-variant);
}
</style>
