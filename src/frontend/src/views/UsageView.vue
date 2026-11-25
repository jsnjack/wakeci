<template>
    <div class="ui analytics-page stack gap-4">
        <AnalyticsTabs />

        <div
            class="cluster gap-3 analytics-filters"
            data-cy="usage-filters"
        >
            <div
                class="segmented"
                role="group"
                aria-label="Days"
            >
                <button
                    v-for="range in ranges"
                    :key="range"
                    type="button"
                    :aria-pressed="days === range ? 'true' : 'false'"
                    :data-cy="`range-${range}`"
                    @click="days = range"
                >
                    {{ range }} days
                </button>
            </div>
            <SearchSelect
                v-model="via"
                :options="viaOptions"
                label="Interface"
                class="filter-select"
                data-cy="via-filter"
            />
            <SearchSelect
                v-model="who"
                :options="whoOptions"
                label="Person or key"
                class="filter-select"
                data-cy="who-filter"
            />
            <span
                v-show="loading"
                class="status-ring spinner"
                aria-label="Loading"
            ></span>
            <span class="grow"></span>
            <small
                v-if="data"
                class="muted"
                data-cy="usage-range"
            >
                {{ rangeText }} · server time
            </small>
        </div>

        <article
            v-if="data && startsLate"
            class="panel note muted"
            data-cy="usage-since"
        >
            <i class="icon small">info</i> Counting started on {{ dayLabel(data.since, true) }}.
        </article>

        <template v-if="data && (data.totals.requests > 0 || data.totals.toolCalls > 0)">
            <div class="cols tiles">
                <article
                    v-for="tile in tiles"
                    :key="tile.key"
                    class="panel tile"
                    :data-cy="`tile-${tile.key}`"
                >
                    <div class="muted tile-label">{{ tile.label }}</div>
                    <div
                        class="tile-value"
                        data-cy="tile-value"
                    >
                        {{ tile.value }}
                    </div>
                    <div class="muted tile-sub">{{ tile.sub }}</div>
                    <SparkLine :values="tile.spark" />
                </article>
            </div>

            <article class="panel chart-card">
                <h2 class="card-title">Requests per day, by outcome</h2>
                <StatusBarsChart
                    :days="data.byDay"
                    :series="outcomes"
                    noun="requests"
                />
                <StatusLegend :series="outcomes" />
                <p class="chart-note">Refused: not signed in, not allowed, or too many requests (401, 403, 429). Client error: other 4xx. Server error: 5xx.</p>
            </article>

            <div class="cols chart-grid">
                <article
                    class="panel"
                    data-cy="interfaces"
                >
                    <h2 class="card-title">Requests by interface</h2>
                    <div
                        v-for="item in data.interfaces"
                        :key="item.via"
                        class="bar-row"
                        :title="outcomeText(item, item.requests)"
                        data-cy="interface"
                    >
                        <span>{{ vias[item.via] || item.via }}</span>
                        <span class="bar"><span :style="{ width: barWidth(item.requests, interfacesTotal) }"></span></span>
                        <span class="muted right-align">{{ item.requests.toLocaleString() }} · {{ formatShare(item.requests, interfacesTotal) }}</span>
                    </div>
                </article>
                <article
                    class="panel"
                    data-cy="people"
                >
                    <h2 class="card-title">People and API keys</h2>
                    <div
                        v-for="person in shownPeople"
                        :key="person.id"
                        class="bar-row"
                        :title="personTitle(person)"
                        data-cy="person"
                    >
                        <span class="ellipsis">
                            <i class="icon small">{{ person.kind === "apikey" ? "key" : "person" }}</i>
                            {{ personName(person) }}
                        </span>
                        <span class="bar"><span :style="{ width: barWidth(person.requests, data.people[0].requests) }"></span></span>
                        <span class="muted right-align">{{ person.requests.toLocaleString() }}</span>
                    </div>
                    <p
                        v-if="data.people.length > shownPeople.length"
                        class="chart-note"
                    >
                        {{ data.people.length - shownPeople.length }} more in the "Person or key" filter.
                    </p>
                </article>
            </div>

            <article
                class="panel"
                data-cy="routes"
            >
                <UsageTable
                    title="Routes"
                    search-label="Filter routes"
                    :rows="data.routes"
                    :name-columns="routeColumns"
                    count-key="requests"
                    count-label="Requests"
                    cy="routes"
                />
                <p class="chart-note">Median and p95 are estimates. Times leave out live updates (/ws) and requests that wait for a build.</p>
            </article>

            <article
                v-if="data.tools.length"
                class="panel"
                data-cy="tools"
            >
                <UsageTable
                    title="MCP tools"
                    search-label="Filter MCP tools"
                    :rows="data.tools"
                    :name-columns="toolColumns"
                    count-key="calls"
                    count-label="Calls"
                    cy="tools"
                />
                <p class="chart-note">Calls with invalid arguments aren't counted here.</p>
            </article>
        </template>

        <article
            v-else-if="data"
            class="panel empty"
            data-cy="usage-empty"
        >
            <div class="stack gap-2 empty-body">
                <i class="icon large">api</i>
                <h2>No requests in these days</h2>
                <p class="muted">Choose a longer range, or another way in or person.</p>
            </div>
        </article>
    </div>
</template>

<script>
import "@/assets/charts.css";
import { OUTCOMES, VIAS, barWidth, dayLabel, formatRate, formatShare, perDay, plural } from "@/analytics";
import AnalyticsTabs from "@/components/AnalyticsTabs.vue";
import SearchSelect from "@/components/SearchSelect.vue";
import SparkLine from "@/components/charts/SparkLine.vue";
import StatusBarsChart from "@/components/charts/StatusBarsChart.vue";
import StatusLegend from "@/components/charts/StatusLegend.vue";
import UsageTable from "@/components/UsageTable.vue";
import axios from "axios";
import vuex from "vuex";

const RANGES = [7, 30, 90, 365];
// How many people and keys the card lists
const PEOPLE_SHOWN = 10;

export default {
    components: { AnalyticsTabs, SearchSelect, SparkLine, StatusBarsChart, StatusLegend, UsageTable },
    data() {
        return {
            ranges: RANGES,
            vias: VIAS,
            outcomes: OUTCOMES,
            days: 30,
            via: "",
            who: "",
            data: null,
            loading: false,
            routeColumns: [
                { key: "via", label: "Interface", format: (row) => VIAS[row.via] || row.via },
                { key: "route", label: "Route", code: true, format: (row) => row.route || "No matching route" },
            ],
            toolColumns: [{ key: "tool", label: "Tool", code: true, format: (row) => row.tool }],
        };
    },
    computed: {
        ...vuex.mapState(["ws"]),
        rangeText() {
            return `${dayLabel(this.data.from, true)} – ${dayLabel(this.data.today, true)}`;
        },
        startsLate() {
            return this.data.since && this.data.since > this.data.from;
        },
        // countedDays are the days of the range that wakeci counted requests
        // on, so the average a day isn't diluted by days before it started
        countedDays() {
            if (!this.startsLate) {
                return this.days;
            }
            return this.data.byDay.filter((d) => d.day >= this.data.since).length || 1;
        },
        viaOptions() {
            return [{ value: "", label: "All" }, ...Object.entries(VIAS).map(([value, label]) => ({ value, label }))];
        },
        whoOptions() {
            const people = this.data ? this.data.people : [];
            const options = people.map((person) => ({ value: person.id, label: this.personName(person) }));
            // The one in the address, until the answer names it
            if (this.who && !options.some((option) => option.value === this.who)) {
                options.push({ value: this.who, label: this.who });
            }
            return [{ value: "", label: "Anyone" }, ...options];
        },
        interfacesTotal() {
            return this.data.interfaces.reduce((sum, item) => sum + item.requests, 0);
        },
        shownPeople() {
            return this.data.people.slice(0, PEOPLE_SHOWN);
        },
        tiles() {
            const t = this.data.totals;
            const days = this.data.byDay;
            const failedCalls = t.toolCalls - t.toolOutcomes.success;
            return [
                {
                    key: "requests",
                    label: "Requests",
                    value: t.requests.toLocaleString(),
                    sub: perDay(t.requests, this.countedDays),
                    spark: days.map((d) => d.requests),
                },
                {
                    key: "success",
                    label: "Success rate",
                    value: formatRate(t.requests ? t.success / t.requests : null, 1),
                    sub: `${t.refused.toLocaleString()} refused · ${plural(t.clientError, "client error")}`,
                    spark: days.map((d) => (d.requests ? d.success / d.requests : null)),
                },
                {
                    key: "serverErrors",
                    label: "Server errors",
                    value: t.serverError.toLocaleString(),
                    sub: t.serverError ? `${formatRate(t.serverError / t.requests, 2)} of requests (5xx)` : "no 5xx answers",
                    spark: days.map((d) => d.serverError),
                },
                {
                    key: "toolCalls",
                    label: "MCP tool calls",
                    value: t.toolCalls.toLocaleString(),
                    sub: t.toolCalls ? `${failedCalls.toLocaleString()} failed` : "none in these days",
                    spark: days.map((d) => d.toolCalls),
                },
            ];
        },
    },
    watch: {
        days: "changed",
        via: "changed",
        who: "changed",
        // Requests made while disconnected
        "ws.reconnects"() {
            this.fetch();
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Analytics");
        const query = this.$route.query;
        const days = Number(query.days);
        if (RANGES.includes(days)) {
            this.days = days;
        }
        this.via = typeof query.via === "string" && VIAS[query.via] ? query.via : "";
        this.who = typeof query.who === "string" ? query.who : "";
        this.fetch();
    },
    methods: {
        barWidth,
        dayLabel,
        formatShare,
        changed() {
            const query = {};
            if (this.days !== 30) {
                query.days = String(this.days);
            }
            if (this.via) {
                query.via = this.via;
            }
            if (this.who) {
                query.who = this.who;
            }
            if (this.$router.resolve({ query }).fullPath !== this.$route.fullPath) {
                this.$router.replace({ query });
            }
            this.fetch();
        },
        fetch() {
            this.loading = true;
            const params = { days: this.days };
            if (this.via) {
                params.via = this.via;
            }
            if (this.who) {
                params.who = this.who;
            }
            const asked = JSON.stringify(params);
            this.asked = asked;
            axios
                .get("/api/analytics/requests", { params })
                .then((response) => {
                    // An answer to an older choice of filters is dropped
                    if (this.asked === asked) {
                        this.data = response.data;
                    }
                })
                .catch(() => {})
                .finally(() => {
                    if (this.asked === asked) {
                        this.loading = false;
                    }
                });
        },
        outcomeText(counts, total) {
            const parts = [`${counts.success.toLocaleString()} succeeded`];
            for (const outcome of OUTCOMES) {
                if (outcome.key !== "success" && counts[outcome.key]) {
                    parts.push(`${counts[outcome.key].toLocaleString()} ${outcome.label}`);
                }
            }
            return `${plural(total, "request")}: ${parts.join(", ")}`;
        },
        personName(person) {
            if (person.kind === "password") {
                return "Admin password";
            }
            if (person.kind === "apikey") {
                const key = person.keyName || `revoked key ${person.apiKey}`;
                return person.user ? `${key} (${person.user})` : key;
            }
            return person.user;
        },
        personTitle(person) {
            const ways = Object.entries(person.byVia)
                .sort((a, b) => b[1] - a[1])
                .map(([via, n]) => `${VIAS[via] || via} ${n.toLocaleString()}`);
            const calls = person.toolCalls ? `; ${person.toolCalls.toLocaleString()} MCP tool calls` : "";
            return `${this.personName(person)}\n${this.outcomeText(person, person.requests)}\n${ways.join(" · ")}${calls}`;
        },
    },
};
</script>

<style scoped>
/* As on the analytics page (AnalyticsView.vue) */
.filter-select {
    inline-size: 14rem;
    max-inline-size: 100%;
}

.spinner {
    inline-size: 18px;
    block-size: 18px;
    --_c: var(--primary);
}

.note {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
}

/* The tiles share the row, however many fit */
.tiles {
    grid-template-columns: repeat(auto-fit, minmax(min(12rem, 100%), 1fr));
}

.chart-grid {
    --min: 26rem;
}

.panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-inline-size: 0;
}

.tile-label {
    font-size: var(--text-sm);
}

.tile-value {
    font-size: 1.75rem;
    font-weight: 600;
    line-height: 1.3;
}

.tile-sub {
    font-size: 0.8rem;
    min-block-size: 1.2rem;
}

.bar-row {
    display: grid;
    grid-template-columns: minmax(8rem, 12rem) 1fr 6rem;
    gap: var(--space-3);
    align-items: center;
    padding: 0.2rem 0;
    font-size: 0.9rem;
}

.bar {
    block-size: 0.6rem;
    border-radius: 0.2rem;
    background: var(--surface-container-highest);
    overflow: hidden;
}

.bar > span {
    display: block;
    block-size: 100%;
    background: var(--chart-1);
}

.ellipsis {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.card-title {
    font-size: var(--text-md);
}

.empty {
    align-items: center;
    padding: var(--space-8) var(--space-4);
}

.empty-body {
    align-items: center;
    text-align: center;
}
</style>
