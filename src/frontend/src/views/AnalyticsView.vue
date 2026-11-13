<template>
    <div class="analytics-page">
        <nav
            class="wrap analytics-filters"
            data-cy="analytics-filters"
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
                v-model="job"
                :options="jobOptions"
                label="Job"
                class="filter-select"
                data-cy="job-filter"
            />
            <SearchSelect
                v-model="trigger"
                :options="triggerOptions"
                label="Started by"
                class="filter-select"
                data-cy="trigger-filter"
            />
            <progress
                v-show="loading"
                class="circle small"
            ></progress>
            <div class="max"></div>
            <small
                v-if="data"
                class="muted"
                data-cy="analytics-range"
            >
                {{ rangeText }}
            </small>
        </nav>

        <article
            v-if="data && startsLate"
            class="border no-elevate small-padding muted"
            data-cy="analytics-since"
        >
            <i>info</i> wakeci keeps analytics since {{ sinceText }}, so the days before have no builds.
        </article>

        <template v-if="data && data.totals.builds > 0">
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
                <h6 class="small no-margin">Builds per day, by result</h6>
                <StatusBarsChart :days="data.byDay" />
                <StatusLegend />
            </article>

            <div class="grid">
                <article class="border no-elevate padding s12 l6 chart-card">
                    <h6 class="small no-margin">Success rate, 7-day rolling</h6>
                    <LineChart
                        :points="ratePoints"
                        :min="rateMin"
                        :max="1"
                        :format="(v) => formatRate(v)"
                        label="Success rate per day, over the 7 days up to it"
                    />
                    <p class="chart-note">
                        Finished builds among finished, failed and timed out ones; aborted builds don't count.<span v-if="rateMin > 0">
                            The axis starts at {{ formatRate(rateMin) }}.</span
                        >
                    </p>
                </article>
                <article class="border no-elevate padding s12 l6 chart-card">
                    <div class="card-head">
                        <h6 class="small no-margin">Duration, 7-day rolling</h6>
                        <SearchSelect
                            v-if="!job && data.jobs.length > 1"
                            :model-value="durationTarget"
                            :options="durationOptions"
                            aria-label="Job whose duration the chart shows"
                            data-cy="duration-job"
                            @update:model-value="durationJob = $event"
                        />
                        <span
                            v-else
                            class="muted ellipsis"
                            >{{ durationTarget }}</span
                        >
                    </div>
                    <!-- The days of the job picked in the chart come in a request of their own -->
                    <LineChart
                        v-if="durationDays"
                        :points="durationPoints"
                        :max="durationMax"
                        :format="axisDuration"
                        label="Median and 95th percentile of build duration"
                    />
                    <div
                        v-else
                        class="chart-loading middle-align center-align"
                    >
                        <progress class="circle small"></progress>
                    </div>
                    <BandLegend />
                </article>
            </div>

            <article class="border no-elevate padding chart-card">
                <h6 class="small no-margin">When builds start</h6>
                <StartsHeatmap :starts="data.starts" />
            </article>

            <div class="grid">
                <article class="border no-elevate padding s12 l6 chart-card">
                    <h6 class="small no-margin">Queue wait, 7-day rolling</h6>
                    <template v-if="data.totals.wait.count > 0">
                        <LineChart
                            :points="waitPoints"
                            :max="waitMax"
                            :format="axisDuration"
                            label="Median and 95th percentile of the time builds waited in the queue"
                        />
                        <BandLegend />
                        <p class="chart-note">
                            From queued to started. A rising 95th percentile means builds wait for a free slot (concurrent builds in Settings).
                        </p>
                    </template>
                    <p
                        v-else
                        class="chart-note"
                    >
                        wakeci records queue times of builds queued from this version on.
                    </p>
                </article>
                <article
                    class="border no-elevate padding s12 l6"
                    data-cy="triggers"
                >
                    <h6 class="small no-margin">Who starts builds</h6>
                    <div
                        v-for="item in data.triggers"
                        :key="item.kind"
                        class="bar-row"
                    >
                        <span>{{ triggers[item.kind] || item.kind }}</span>
                        <span class="bar"><span :style="{ width: share(item.builds, data.totals.builds) }"></span></span>
                        <span class="muted right-align">{{ item.builds }} · {{ share(item.builds, data.totals.builds) }}</span>
                    </div>
                    <template v-if="data.showsPeople && data.people.length">
                        <div class="small-space"></div>
                        <div class="muted small-text">People and API keys</div>
                        <div
                            v-for="person in data.people"
                            :key="person.kind + person.user + person.apiKey"
                            class="bar-row"
                            data-cy="trigger-person"
                        >
                            <span class="ellipsis">
                                <i class="tiny">{{ person.kind === "apikey" ? "key" : "person" }}</i>
                                {{ personName(person) }}
                            </span>
                            <span class="bar"><span :style="{ width: share(person.builds, data.people[0].builds) }"></span></span>
                            <span class="muted right-align">{{ person.builds }}</span>
                        </div>
                    </template>
                    <p
                        v-else-if="!data.showsPeople"
                        class="chart-note"
                    >
                        Only the admin sees which people and API keys start builds.
                    </p>
                </article>
            </div>

            <article
                class="border no-elevate padding"
                data-cy="jobs-table"
            >
                <h6 class="small no-margin">Jobs</h6>
                <div class="table-scroll">
                    <table class="small-text">
                        <thead>
                            <tr>
                                <th
                                    v-for="column in columns"
                                    :key="column.key"
                                    :class="{ 'right-align': column.numeric, sortable: column.sort }"
                                    :aria-sort="sortKey === column.key ? (sortAsc ? 'ascending' : 'descending') : null"
                                    @click="column.sort && sortBy(column.key)"
                                >
                                    {{ column.label }}
                                    <i
                                        v-if="sortKey === column.key"
                                        class="tiny"
                                        >{{ sortAsc ? "arrow_upward" : "arrow_downward" }}</i
                                    >
                                </th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr
                                v-for="row in sortedJobs"
                                :key="row.name"
                                :data-cy="`job-row-${row.name}`"
                            >
                                <td>
                                    <router-link
                                        :to="insightsLink(row.name)"
                                        class="link"
                                        >{{ row.name }}</router-link
                                    >
                                </td>
                                <td><OutcomeStrip :builds="row.recent" /></td>
                                <td
                                    class="right-align"
                                    data-cy="job-success"
                                >
                                    {{ formatRate(row.successRate) }}
                                </td>
                                <td class="right-align">{{ row.builds }}</td>
                                <td class="right-align">{{ row.duration.count ? formatDuration(row.duration.p50) : "—" }}</td>
                                <td class="right-align">{{ row.duration.count ? formatDuration(row.duration.p95) : "—" }}</td>
                                <td class="right-align nowrap">
                                    <span class="share-bar"><span :style="{ width: share(row.buildTime, totalBuildTime) }"></span></span>
                                    {{ share(row.buildTime, totalBuildTime) }}
                                </td>
                                <td class="nowrap">{{ ago(lastStarted(row)) }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
                <StatusLegend />
                <p class="chart-note">
                    A short bar is a finished build, a full one failed or timed out. Click a bar to open the build, or a job to see its tasks.
                </p>
            </article>
        </template>

        <article
            v-else-if="data"
            class="border no-elevate medium-height middle-align center-align"
            data-cy="analytics-empty"
        >
            <div class="center-align">
                <i class="extra">monitoring</i>
                <h5>No builds in these days</h5>
                <p class="muted">Choose a longer range, or another job or trigger.</p>
            </div>
        </article>
    </div>
</template>

<script>
import "@/assets/charts.css";
import { STATUSES, TRIGGERS, ago, axisDuration, dayLabel, formatDuration, formatRate, isZeroTime, niceDurationMax } from "@/analytics";
import BandLegend from "@/components/charts/BandLegend.vue";
import LineChart from "@/components/charts/LineChart.vue";
import OutcomeStrip from "@/components/charts/OutcomeStrip.vue";
import SearchSelect from "@/components/SearchSelect.vue";
import SparkLine from "@/components/charts/SparkLine.vue";
import StartsHeatmap from "@/components/charts/StartsHeatmap.vue";
import StatusBarsChart from "@/components/charts/StatusBarsChart.vue";
import StatusLegend from "@/components/charts/StatusLegend.vue";
import axios from "axios";
import debounce from "lodash/debounce";
import vuex from "vuex";

const RANGES = [7, 30, 90, 365];
const FINAL = new Set(STATUSES.map((s) => s.status));

export default {
    components: { BandLegend, LineChart, OutcomeStrip, SearchSelect, SparkLine, StartsHeatmap, StatusBarsChart, StatusLegend },
    data() {
        return {
            ranges: RANGES,
            triggers: TRIGGERS,
            days: 30,
            job: "",
            trigger: "",
            jobNames: [],
            data: null,
            // The job the duration chart shows, if picked there, and its days
            durationJob: "",
            durationData: null,
            loading: false,
            // The jobs table's order; null keeps the API's, least reliable first
            sortKey: null,
            sortAsc: true,
            subscription: "build:update:",
            columns: [
                { key: "name", label: "Job", sort: true },
                { key: "recent", label: "Last builds" },
                { key: "successRate", label: "Success", numeric: true, sort: true },
                { key: "builds", label: "Builds", numeric: true, sort: true },
                { key: "p50", label: "Median", numeric: true, sort: true },
                { key: "p95", label: "p95", numeric: true, sort: true },
                { key: "buildTime", label: "Build time", numeric: true, sort: true },
                { key: "last", label: "Last build", sort: true },
            ],
        };
    },
    computed: {
        ...vuex.mapState(["ws"]),
        rangeText() {
            return `${dayLabel(this.data.from, true)} – ${dayLabel(this.data.today, true)}`;
        },
        startsLate() {
            return this.data.since && new Date(this.data.since) > new Date(`${this.data.from}T00:00:00`);
        },
        sinceText() {
            return new Date(this.data.since).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
        },
        tiles() {
            const t = this.data.totals;
            const days = this.data.byDay;
            let rateSub = `of ${t.finished + t.failed + t.timedOut} that ran to the end`;
            if (t.successRate !== null && this.data.previousSuccessRate !== null) {
                const diff = (t.successRate - this.data.previousSuccessRate) * 100;
                rateSub = `${diff >= 0 ? "▲" : "▼"} ${Math.abs(diff).toFixed(1)} pts against the ${this.days} days before`;
            }
            return [
                {
                    key: "builds",
                    label: "Builds",
                    value: t.builds.toLocaleString(),
                    sub: this.days > 1 ? `about ${Math.round(t.builds / this.days).toLocaleString()} a day` : "today",
                    spark: days.map((d) => d.finished + d.failed + d.timedOut + d.aborted),
                },
                {
                    key: "success",
                    label: "Success rate",
                    value: formatRate(t.successRate, 1),
                    sub: rateSub,
                    spark: days.map((d) => d.successRate),
                },
                {
                    key: "duration",
                    label: "Median duration",
                    value: t.duration.count ? formatDuration(t.duration.p50) : "—",
                    sub: t.duration.count ? `p95 ${formatDuration(t.duration.p95)} · finished builds` : "no finished builds",
                    spark: days.map((d) => (d.duration.count ? d.duration.p50 : null)),
                },
                {
                    key: "wait",
                    label: "Queue wait",
                    value: t.wait.count ? formatDuration(t.wait.p50) : "—",
                    sub: t.wait.count ? `median · p95 ${formatDuration(t.wait.p95)}` : "recorded from this version on",
                    spark: days.map((d) => (d.wait.count ? d.wait.p50 : null)),
                },
            ];
        },
        ratePoints() {
            return this.data.byDay.map((d) => ({
                label: dayLabel(d.day),
                value: d.successRate,
                title: `7 days to ${dayLabel(d.day, true)}: ${formatRate(d.successRate, 1)}`,
            }));
        },
        rateMin() {
            const rates = this.data.byDay.map((d) => d.successRate).filter((r) => r !== null);
            if (!rates.length) {
                return 0;
            }
            return Math.max(0, Math.min(0.9, Math.floor((Math.min(...rates) - 0.05) * 10) / 10));
        },
        jobOptions() {
            return [{ value: "", label: "All jobs" }, ...this.jobNames.map((name) => ({ value: name, label: name }))];
        },
        triggerOptions() {
            return [{ value: "", label: "Anyone" }, ...Object.entries(TRIGGERS).map(([value, label]) => ({ value, label }))];
        },
        durationOptions() {
            return this.jobsByBuildTime.map((row) => ({ value: row.name, label: row.name }));
        },
        // The duration chart shows one job, since jobs of seconds and of
        // hours make a median of all of them say little: the job filtered
        // to, the one picked in the chart, or the one that runs longest
        jobsByBuildTime() {
            return [...this.data.jobs].sort((a, b) => b.buildTime - a.buildTime);
        },
        durationTarget() {
            if (this.job) {
                return this.job;
            }
            if (this.data.jobs.some((j) => j.name === this.durationJob)) {
                return this.durationJob;
            }
            return this.jobsByBuildTime.length ? this.jobsByBuildTime[0].name : "";
        },
        durationDays() {
            if (this.job) {
                return this.data.byDay;
            }
            // Null until the days of this job, range and trigger came
            if (!this.durationData || this.durationData.params !== this.durationParams) {
                return null;
            }
            return this.durationData.byDay;
        },
        // What the duration chart asks for, as a key
        durationParams() {
            const params = { days: this.days, job: this.durationTarget };
            if (this.trigger) {
                params.trigger = this.trigger;
            }
            return JSON.stringify(params);
        },
        durationPoints() {
            return this.bandPoints("duration", this.durationDays);
        },
        durationMax() {
            return niceDurationMax(Math.max(1, ...this.durationDays.map((d) => d.duration.p95)));
        },
        waitPoints() {
            return this.bandPoints("wait", this.data.byDay);
        },
        waitMax() {
            return niceDurationMax(Math.max(1, ...this.data.byDay.map((d) => d.wait.p95)));
        },
        totalBuildTime() {
            return this.data.jobs.reduce((sum, job) => sum + job.buildTime, 0);
        },
        sortedJobs() {
            if (!this.sortKey) {
                return this.data.jobs;
            }
            const value = (job) => {
                switch (this.sortKey) {
                    case "p50":
                        return job.duration.count ? job.duration.p50 : -1;
                    case "p95":
                        return job.duration.count ? job.duration.p95 : -1;
                    case "last":
                        return this.lastStarted(job);
                    case "successRate":
                        return job.successRate === null ? 2 : job.successRate;
                    default:
                        return job[this.sortKey];
                }
            };
            const sorted = [...this.data.jobs].sort((a, b) => {
                const x = value(a);
                const y = value(b);
                return typeof x === "string" ? x.localeCompare(y) : x - y;
            });
            return this.sortAsc ? sorted : sorted.reverse();
        },
    },
    watch: {
        days: "changed",
        job: "changed",
        trigger: "changed",
        durationJob: "fetchDuration",
        "ws.connected"(connected) {
            if (connected) {
                this.subscribe();
            }
        },
        // Jobs the user may now read, and builds that ended while
        // disconnected
        "ws.reconnects"() {
            this.fetch();
        },
    },
    created() {
        // Builds end in bursts; one fetch covers them
        this.refresh = debounce(() => this.fetch(), 3000, { maxWait: 15000 });
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Analytics");
        const query = this.$route.query;
        const days = Number(query.days);
        if (RANGES.includes(days)) {
            this.days = days;
        }
        this.job = typeof query.job === "string" ? query.job : "";
        this.trigger = typeof query.trigger === "string" && TRIGGERS[query.trigger] ? query.trigger : "";
        this.fetch();
        this.fetchJobs();
        this.subscribe();
        this.emitter.on(this.subscription, this.onBuildUpdate);
    },
    unmounted() {
        this.refresh.cancel();
        this.emitter.off(this.subscription, this.onBuildUpdate);
        this.$store.commit("WS_SEND", { type: "in:unsubscribe", data: { to: [this.subscription] } });
    },
    methods: {
        ago,
        axisDuration,
        formatDuration,
        formatRate,
        changed() {
            const query = {};
            if (this.days !== 30) {
                query.days = String(this.days);
            }
            if (this.job) {
                query.job = this.job;
            }
            if (this.trigger) {
                query.trigger = this.trigger;
            }
            if (this.$router.resolve({ query }).fullPath !== this.$route.fullPath) {
                this.$router.replace({ query });
            }
            this.fetch();
        },
        fetch() {
            this.loading = true;
            const params = { days: this.days };
            if (this.job) {
                params.job = this.job;
            }
            if (this.trigger) {
                params.trigger = this.trigger;
            }
            const asked = JSON.stringify(params);
            this.asked = asked;
            axios
                .get("/api/analytics/", { params })
                .then((response) => {
                    // An answer to an older choice of filters is dropped
                    if (this.asked === asked) {
                        this.data = response.data;
                        this.fetchDuration();
                    }
                })
                .catch(() => {})
                .finally(() => {
                    if (this.asked === asked) {
                        this.loading = false;
                    }
                });
        },
        // fetchDuration reads the days of the job the duration chart shows,
        // unless the page is filtered to a job already
        fetchDuration() {
            if (this.job || !this.durationTarget) {
                return;
            }
            const params = this.durationParams;
            axios
                .get("/api/analytics/", { params: JSON.parse(params) })
                .then((response) => {
                    // An answer for other choices is dropped
                    if (params === this.durationParams) {
                        this.durationData = { params, byDay: response.data.byDay };
                    }
                })
                .catch(() => {});
        },
        fetchJobs() {
            axios
                .get("/api/jobs/")
                .then((response) => {
                    this.jobNames = (response.data || []).map((job) => job.name).sort();
                })
                .catch(() => {});
        },
        subscribe() {
            this.$store.commit("WS_SEND", { type: "in:subscribe", data: { to: [this.subscription] } });
        },
        onBuildUpdate(build) {
            // A build counts once it ended; its duration is set then
            if (FINAL.has(build.status) && build.duration > 0) {
                this.refresh();
            }
        },
        bandPoints(key, days) {
            return days.map((d) => {
                const p = d[key];
                return {
                    label: dayLabel(d.day),
                    value: p.count ? p.p50 : null,
                    upper: p.count ? p.p95 : null,
                    title: p.count
                        ? `7 days to ${dayLabel(d.day, true)}: median ${formatDuration(p.p50)}, p95 ${formatDuration(p.p95)} (${p.count} builds)`
                        : `7 days to ${dayLabel(d.day, true)}: no builds`,
                };
            });
        },
        share(part, whole) {
            if (!whole) {
                return "0%";
            }
            return `${Math.round((part / whole) * 100)}%`;
        },
        personName(person) {
            if (person.kind === "apikey") {
                const key = person.keyName || `revoked key ${person.apiKey}`;
                return person.user ? `${key} (${person.user})` : key;
            }
            return person.user;
        },
        lastStarted(job) {
            for (let i = job.recent.length - 1; i >= 0; i--) {
                if (!isZeroTime(job.recent[i].startedAt)) {
                    return job.recent[i].startedAt;
                }
            }
            return "";
        },
        sortBy(key) {
            if (this.sortKey === key) {
                this.sortAsc = !this.sortAsc;
            } else {
                this.sortKey = key;
                this.sortAsc = key === "name" || key === "successRate";
            }
        },
        insightsLink(name) {
            return { name: "jobInsights", params: { name } };
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
/* A fixed width, since BeerCSS doesn't let a nav's children shrink; the
   picker cuts a long job name with an ellipsis */
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
.bar,
.share-bar {
    height: 0.6rem;
    border-radius: 0.2rem;
    background: var(--surface-container-highest);
    overflow: hidden;
}
.bar > span,
.share-bar > span {
    display: block;
    height: 100%;
    background: var(--chart-1);
}
.share-bar {
    display: inline-block;
    width: 3.5rem;
    vertical-align: middle;
    margin-right: 0.4rem;
}
.table-scroll {
    overflow-x: auto;
}
th.sortable {
    cursor: pointer;
    user-select: none;
}
.nowrap {
    white-space: nowrap;
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
.chart-loading {
    block-size: 160px;
}
.card-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem 1rem;
}
.card-head > h6 {
    flex: 1 1 auto;
}
.card-head > :not(h6) {
    flex: 0 1 16rem;
    min-width: 8rem;
    max-width: 100%;
}
</style>
