<template>
    <div class="analytics-page">
        <AnalyticsTabs />

        <nav
            class="wrap analytics-filters"
            data-cy="usage-filters"
        >
            <nav class="group connected">
                <button
                    v-for="(range, i) in ranges"
                    :key="range"
                    :class="{
                        active: days === range,
                        'left-round': i === 0,
                        'right-round': i === ranges.length - 1,
                        'no-round': i > 0 && i < ranges.length - 1,
                    }"
                    :data-cy="`range-${range}`"
                    @click.prevent="days = range"
                >
                    {{ range }} days
                </button>
            </nav>
            <SearchSelect
                v-model="via"
                :options="viaOptions"
                label="Came in through"
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
            <progress
                v-show="loading"
                class="circle small"
            ></progress>
            <div class="max"></div>
            <small
                v-if="data"
                class="muted"
                data-cy="usage-range"
            >
                {{ rangeText }} · server time
            </small>
        </nav>

        <article
            v-if="data && startsLate"
            class="border no-elevate small-padding muted"
            data-cy="usage-since"
        >
            <i>info</i> wakeci counts requests since {{ dayLabel(data.since, true) }}, so the days before have none.
        </article>

        <template v-if="data && (data.totals.requests > 0 || data.totals.toolCalls > 0)">
            <div class="grid tiles">
                <article
                    v-for="tile in tiles"
                    :key="tile.key"
                    class="border no-elevate padding s6 l3 tile"
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

            <article class="border no-elevate padding chart-card">
                <h6 class="small no-margin">Requests per day, by outcome</h6>
                <StatusBarsChart
                    :days="data.byDay"
                    :series="outcomes"
                    noun="requests"
                />
                <StatusLegend :series="outcomes" />
                <p class="chart-note">
                    Refused: wrong or missing credentials, no permission, or too many requests (401, 403, 429). Client error: any other 4xx, such as 400 or 404.
                    Server error: 5xx.
                </p>
            </article>

            <div class="grid">
                <article
                    class="border no-elevate padding s12 l6"
                    data-cy="interfaces"
                >
                    <h6 class="small no-margin">How requests come in</h6>
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
                    <p class="chart-note">
                        By the credentials a request carried, right or wrong: a session is the web UI, Basic auth with the password and API keys are the API.
                        MCP counts the requests to /mcp; each tool call is one of them.
                    </p>
                </article>
                <article
                    class="border no-elevate padding s12 l6"
                    data-cy="people"
                >
                    <h6 class="small no-margin">People and API keys</h6>
                    <div
                        v-for="person in shownPeople"
                        :key="person.id"
                        class="bar-row"
                        :title="personTitle(person)"
                        data-cy="person"
                    >
                        <span class="ellipsis">
                            <i class="tiny">{{ person.kind === "apikey" ? "key" : "person" }}</i>
                            {{ personName(person) }}
                        </span>
                        <span class="bar"><span :style="{ width: barWidth(person.requests, data.people[0].requests) }"></span></span>
                        <span class="muted right-align">{{ person.requests.toLocaleString() }}</span>
                    </div>
                    <p
                        v-if="data.people.length > shownPeople.length"
                        class="chart-note"
                    >
                        And {{ data.people.length - shownPeople.length }} more; pick one in "Person or key".
                    </p>
                    <p class="chart-note">Requests whose credentials were missing or wrong have no one.</p>
                </article>
            </div>

            <article
                class="border no-elevate padding"
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
                <p class="chart-note">
                    "No route" is a path that no route matches; for API keys, also a route that keys can't use. Median and p95 are estimated from time buckets,
                    so they are within about a third; the slowest is exact. WebSockets (/ws) and reads that wait for a build (wait=) are counted, but their time
                    isn't. A download from /storage takes as long as its file.
                </p>
            </article>

            <article
                v-if="data.tools.length"
                class="border no-elevate padding"
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
                <p class="chart-note">
                    A failed call counts with the status of the API call that failed in it. A call whose arguments don't fit the tool is turned down before it
                    runs and counts only as a request to /mcp.
                </p>
            </article>
        </template>

        <article
            v-else-if="data"
            class="border no-elevate medium-height middle-align center-align"
            data-cy="usage-empty"
        >
            <div class="center-align">
                <i class="extra">api</i>
                <h5>No requests in these days</h5>
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
                { key: "via", label: "Came in through", format: (row) => VIAS[row.via] || row.via },
                { key: "route", label: "Route", code: true, format: (row) => row.route || "No route" },
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
        viaOptions() {
            return [{ value: "", label: "Any way" }, ...Object.entries(VIAS).map(([value, label]) => ({ value, label }))];
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
                    sub: perDay(t.requests, this.days),
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
            return `${total.toLocaleString()} requests: ${parts.join(", ")}`;
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
            return `${this.outcomeText(person, person.requests)}\n${ways.join(" · ")}${calls}`;
        },
    },
};
</script>

<style scoped>
.analytics-filters {
    gap: 0.75rem;
    align-items: center;
    margin: 0;
}
.filter-select {
    width: 14rem;
    max-width: 100%;
}
.tiles {
    margin-bottom: 0;
}
.tile-label {
    font-size: 0.85rem;
}
.tile-value {
    font-size: 1.75rem;
    font-weight: 600;
    line-height: 1.3;
}
.tile-sub {
    font-size: 0.8rem;
    min-height: 1.2rem;
}
.bar-row {
    display: grid;
    grid-template-columns: minmax(8rem, 12rem) 1fr 6rem;
    gap: 0.75rem;
    align-items: center;
    padding: 0.2rem 0;
    font-size: 0.9rem;
}
.bar {
    height: 0.6rem;
    border-radius: 0.2rem;
    background: var(--surface-container-highest);
    overflow: hidden;
}
.bar > span {
    display: block;
    height: 100%;
    background: var(--chart-1);
}
.muted {
    color: var(--on-surface-variant);
}
.ellipsis {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.analytics-page {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}
.analytics-page > .grid {
    margin: 0;
}
</style>
