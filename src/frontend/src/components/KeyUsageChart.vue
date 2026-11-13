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
            <line
                class="baseline"
                x1="0"
                :x2="width"
                :y1="height - 0.5"
                :y2="height - 0.5"
            />
            <g
                v-for="(bar, i) in bars"
                :key="bar.day"
                data-cy="key-usage-bar"
            >
                <title>{{ bar.label }}: {{ requests(bar.uses) }}</title>
                <!-- The whole column shows the tooltip, also on a day without requests -->
                <rect
                    class="hover"
                    :x="i * step"
                    y="0"
                    :width="barWidth"
                    :height="height"
                />
                <rect
                    v-if="bar.height > 0"
                    class="bar"
                    :x="i * step"
                    :y="height - bar.height"
                    :width="barWidth"
                    :height="bar.height"
                />
            </g>
        </svg>
        <span data-cy="key-usage-total">{{ caption }}</span>
    </span>
</template>

<script>
// KeyUsageChart shows an API key's requests per day, today last, as tiny
// bars over a baseline; hovering a bar names its day and count. It starts on
// the day the key was created, at most 30 days back, so a young key gets
// fewer, wider bars instead of days it didn't exist. Days are the server's,
// so the chart ends on the day the server says is today.
export default {
    props: {
        // The days with requests, oldest first: [{ day: "2026-11-12", uses: 3 }]
        usesByDay: {
            type: Array,
            default: () => [],
        },
        // The server's date
        today: {
            type: String,
            required: true,
        },
        // When the key was created. The server sends it with its own offset,
        // so its date is the server's day, as in usesByDay.
        created: {
            type: String,
            default: "",
        },
    },
    data: function () {
        return { maxDays: 30, maxWidth: 120, maxStep: 12, height: 18 };
    },
    computed: {
        // The first day to show: the key's first day, or the first one with
        // requests should the two disagree
        firstDay() {
            const days = [this.created.slice(0, 10)];
            if (this.usesByDay.length > 0) {
                days.push(this.usesByDay[0].day);
            }
            return days.filter((day) => !Number.isNaN(Date.parse(day + "T00:00:00Z"))).sort()[0] || "";
        },
        dayCount() {
            const days = Math.round((Date.parse(this.today + "T00:00:00Z") - Date.parse(this.firstDay + "T00:00:00Z")) / 86400000) + 1;
            return Number.isFinite(days) ? Math.min(this.maxDays, Math.max(1, days)) : this.maxDays;
        },
        // 4 px per day over 30 days, up to 12 for a young key
        step() {
            return Math.min(this.maxStep, Math.floor(this.maxWidth / this.dayCount));
        },
        barWidth() {
            return Math.max(3, this.step - 2);
        },
        width() {
            return (this.dayCount - 1) * this.step + this.barWidth;
        },
        bars() {
            const uses = new Map(this.usesByDay.map((d) => [d.day, d.uses]));
            const end = new Date(this.today + "T00:00:00Z");
            const days = [];
            for (let ago = this.dayCount - 1; ago >= 0; ago--) {
                const date = new Date(end);
                date.setUTCDate(end.getUTCDate() - ago);
                const day = date.toISOString().slice(0, 10);
                days.push({ day, uses: uses.get(day) || 0, label: this.dayLabel(day) });
            }
            const max = Math.max(1, ...days.map((d) => d.uses));
            // 3 px at least, so a quiet day stands out from the 1 px baseline
            return days.map((d) => ({ ...d, height: d.uses > 0 ? Math.max(3, Math.round((d.uses / max) * this.height)) : 0 }));
        },
        total() {
            return this.bars.reduce((sum, bar) => sum + bar.uses, 0);
        },
        // What the bars cover. A key younger than 30 days names its first day
        // rather than a total, which would only repeat the count of all its
        // requests next to the chart.
        period() {
            if (this.dayCount >= this.maxDays) {
                return `in the last ${this.maxDays} days`;
            }
            return this.dayCount === 1 ? "today" : `since ${this.dayLabel(this.firstDay)}`;
        },
        caption() {
            return this.dayCount >= this.maxDays ? `${this.total.toLocaleString()} in ${this.maxDays} days` : this.period;
        },
        summary() {
            if (this.total === 0) {
                return `No requests ${this.period}`;
            }
            const busiest = this.bars.reduce((a, b) => (b.uses > a.uses ? b : a));
            return `${this.requests(this.total)} ${this.period}, most on ${busiest.label} (${busiest.uses})`;
        },
    },
    methods: {
        requests(n) {
            return `${n.toLocaleString()} ${n === 1 ? "request" : "requests"}`;
        },
        dayLabel(day) {
            return new Date(day + "T00:00:00Z").toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" });
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
/* BeerCSS makes every element inherit border-radius, so the SVG took the
   rounded corners of the key's row and clipped the first and last bars */
svg {
    border-radius: 0;
}
.baseline {
    stroke: var(--outline-variant);
    stroke-width: 1;
}
.hover {
    fill: transparent;
}
.bar {
    fill: var(--primary);
}
</style>
