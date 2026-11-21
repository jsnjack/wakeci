<template>
    <div class="ui analytics-page stack gap-4">
        <AnalyticsTabs />

        <div
            class="cluster gap-3 analytics-filters"
            data-cy="analytics-filters"
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
            <span
                v-show="loading"
                class="status-ring spinner"
                aria-label="Loading"
            ></span>
            <span class="grow"></span>
            <small
                v-if="data"
                class="muted"
                data-cy="analytics-range"
            >
                {{ rangeText }}
            </small>
        </div>

        <article
            v-if="data && startsLate"
            class="panel note muted"
            data-cy="analytics-since"
        >
            <i class="icon small">info</i> wakeci keeps analytics since {{ sinceText }}, so the days before have no builds.
        </article>

        <template v-if="data && data.totals.builds > 0">
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
                <h2 class="card-title">Builds per day, by result</h2>
                <StatusBarsChart :days="data.byDay" />
                <StatusLegend />
            </article>

            <div class="cols chart-grid">
                <article class="panel chart-card">
                    <h2 class="card-title">Success rate, 7-day rolling</h2>
                    <LineChart
                        :points="ratePoints"
                        :min="rateMin"
                        :max="1"
                        :format="(v) => formatRate(v)"
                        label="Success rate per day, over the 7 days up to it"
                    />
                    <p class="chart-note">
                        Share of builds that succeeded. Aborted builds don't count.<span v-if="rateMin > 0">
                            The axis starts at {{ formatRate(rateMin) }}.</span
                        >
                    </p>
                </article>
                <article
                    class="panel chart-card"
                    data-cy="duration-card"
                >
                    <div class="card-head">
                        <h2 class="card-title">Duration, 7-day rolling</h2>
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
                        class="chart-loading"
                    >
                        <span
                            class="status-ring spinner"
                            aria-label="Loading"
                        ></span>
                    </div>
                    <BandLegend />
                    <p class="chart-note">Successful builds only.</p>
                </article>
            </div>

            <article class="panel chart-card">
                <h2 class="card-title">When builds start (server time)</h2>
                <StartsHeatmap :starts="data.starts" />
            </article>

            <div class="cols chart-grid">
                <article class="panel chart-card">
                    <h2 class="card-title">Queue wait, 7-day rolling</h2>
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
                        No queue times for these builds: wakeci started recording them in mid-November 2026.
                    </p>
                </article>
                <article
                    class="panel"
                    data-cy="triggers"
                >
                    <h2 class="card-title">Who starts builds</h2>
                    <div
                        v-for="item in data.triggers"
                        :key="item.kind"
                        class="bar-row"
                    >
                        <span>{{ triggers[item.kind] || item.kind }}</span>
                        <span class="bar"><span :style="{ width: barWidth(item.builds, data.totals.builds) }"></span></span>
                        <span class="muted right-align">{{ item.builds }} · {{ formatShare(item.builds, data.totals.builds) }}</span>
                    </div>
                    <template v-if="data.showsPeople && data.people.length">
                        <div class="muted text-sm">People and API keys</div>
                        <div
                            v-for="person in data.people"
                            :key="person.kind + person.user + person.apiKey"
                            class="bar-row"
                            :title="`${personName(person)}: ${plural(person.builds, 'build')}`"
                            data-cy="trigger-person"
                        >
                            <span class="ellipsis">
                                <i class="icon small">{{ person.kind === "apikey" ? "key" : "person" }}</i>
                                {{ personName(person) }}
                            </span>
                            <span class="bar"><span :style="{ width: barWidth(person.builds, data.people[0].builds) }"></span></span>
                            <span class="muted right-align">{{ person.builds }}</span>
                        </div>
                    </template>
                    <p
                        v-else-if="!data.showsPeople"
                        class="chart-note"
                    >
                        Only those who can edit jobs see which people and API keys start builds.
                    </p>
                </article>
            </div>

            <article
                class="panel"
                data-cy="jobs-table"
            >
                <div class="card-head">
                    <h2 class="card-title">Jobs</h2>
                    <label
                        v-if="data.jobs.length > 1"
                        class="input-icon"
                    >
                        <i class="icon small">search</i>
                        <input
                            v-model="jobSearch"
                            type="search"
                            class="input"
                            aria-label="Filter jobs"
                            placeholder="Filter jobs"
                            data-cy="jobs-table-search"
                        />
                    </label>
                </div>
                <div class="table-scroll">
                    <table class="jobs-table text-sm">
                        <thead>
                            <tr>
                                <th
                                    v-for="column in columns"
                                    :key="column.key"
                                    :class="{ 'right-align': column.numeric, sortable: column.sort }"
                                    :title="column.title"
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
                                v-for="row in shownJobs"
                                :key="row.name"
                                :data-cy="`job-row-${row.name}`"
                            >
                                <td>
                                    <router-link :to="insightsLink(row.name)">{{ row.name }}</router-link>
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
                                <td
                                    class="right-align nowrap"
                                    :title="`${formatShare(row.buildTime, totalBuildTime, true)} of all jobs' ${formatDuration(totalBuildTime)}`"
                                    data-cy="job-build-time"
                                >
                                    <span class="share-bar"><span :style="{ width: barWidth(row.buildTime, totalBuildTime) }"></span></span>
                                    <span class="build-time">{{ formatDuration(row.buildTime) }}</span>
                                </td>
                                <td class="nowrap">{{ ago(lastStarted(row)) }}</td>
                            </tr>
                            <tr v-if="shownJobs.length === 0">
                                <td
                                    :colspan="columns.length"
                                    class="muted"
                                    data-cy="jobs-table-none"
                                >
                                    No job matches "{{ jobSearch.trim() }}"
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
                <StatusLegend />
                <p class="chart-note">
                    A short bar is a finished build, a full one failed or timed out. Click a bar to open the build, or a job to see its tasks. The build time
                    bar is the job's share of all jobs.
                </p>
            </article>
        </template>

        <article
            v-else-if="data"
            class="panel empty"
            data-cy="analytics-empty"
        >
            <div class="stack gap-2 empty-body">
                <i class="icon large">monitoring</i>
                <h2>No builds in these days</h2>
                <p class="muted">Choose a longer range, or another job or trigger.</p>
            </div>
        </article>
    </div>
</template>

<script>
import "@/assets/charts.css";
import {
    STATUSES,
    TRIGGERS,
    ago,
    axisDuration,
    barWidth,
    dayLabel,
    formatDuration,
    formatRate,
    formatShare,
    isZeroTime,
    niceDurationMax,
    perDay,
    plural,
} from "@/analytics";
import AnalyticsTabs from "@/components/AnalyticsTabs.vue";
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
    components: { AnalyticsTabs, BandLegend, LineChart, OutcomeStrip, SearchSelect, SparkLine, StartsHeatmap, StatusBarsChart, StatusLegend },
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
            // Words the jobs table's names must all contain
            jobSearch: "",
            subscription: "build:update:",
            columns: [
                { key: "name", label: "Job", sort: true },
                { key: "recent", label: "Last builds" },
                { key: "successRate", label: "Success", numeric: true, sort: true },
                { key: "builds", label: "Builds", numeric: true, sort: true },
                { key: "p50", label: "Median", numeric: true, sort: true },
                { key: "p95", label: "p95", numeric: true, sort: true },
                {
                    key: "buildTime",
                    label: "Build time",
                    title: "Total time the job's builds ran, failed ones included",
                    numeric: true,
                    sort: true,
                },
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
        // countedDays are the days of the range that wakeci has build stats
        // of, so the average a day isn't diluted by days before them
        countedDays() {
            if (!this.startsLate) {
                return this.days;
            }
            const since = new Date(this.data.since);
            return this.data.byDay.filter((d) => new Date(`${d.day}T23:59:59.999`) >= since).length || 1;
        },
        sinceText() {
            return new Date(this.data.since).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
        },
        tiles() {
            const t = this.data.totals;
            const days = this.data.byDay;
            let rateSub = `of ${plural(t.finished + t.failed + t.timedOut, "build")}, not counting aborted`;
            if (t.successRate !== null && this.data.previousSuccessRate !== null) {
                const diff = (t.successRate - this.data.previousSuccessRate) * 100;
                rateSub = `${diff >= 0 ? "▲" : "▼"} ${Math.abs(diff).toFixed(1)} percentage points vs the previous ${this.days} days`;
            }
            return [
                {
                    key: "builds",
                    label: "Builds",
                    value: t.builds.toLocaleString(),
                    sub: perDay(t.builds, this.countedDays),
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
                    sub: t.duration.count ? `p95 ${formatDuration(t.duration.p95)} · successful builds` : "no successful builds",
                    spark: days.map((d) => (d.duration.count ? d.duration.p50 : null)),
                },
                {
                    key: "wait",
                    label: "Queue wait",
                    value: t.wait.count ? formatDuration(t.wait.p50) : "—",
                    sub: t.wait.count ? `median · p95 ${formatDuration(t.wait.p95)}` : "not recorded for these builds",
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
        // The build time shares stay those of all jobs
        shownJobs() {
            const words = this.jobSearch.toLowerCase().split(/\s+/).filter(Boolean);
            return this.sortedJobs.filter((job) => words.every((word) => job.name.toLowerCase().includes(word)));
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
        barWidth,
        formatShare,
        plural,
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
/* A fixed width; the picker cuts a long job name with an ellipsis */
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

.bar,
.share-bar {
    block-size: 0.6rem;
    border-radius: 0.2rem;
    background: var(--surface-container-highest);
    overflow: hidden;
}

.bar > span,
.share-bar > span {
    display: block;
    block-size: 100%;
    background: var(--chart-1);
}

.share-bar {
    display: inline-block;
    inline-size: 3.5rem;
    vertical-align: middle;
    margin-inline-end: 0.4rem;
}
/* As wide as the longest time, "87600h 00m", so the bars before it line up
   whatever the time */
.build-time {
    display: inline-block;
    min-width: 10ch;
    text-align: right;
    font-variant-numeric: tabular-nums;
}

.table-scroll {
    overflow-x: auto;
}

.jobs-table {
    inline-size: 100%;
    border-collapse: collapse;
}

.jobs-table th,
.jobs-table td {
    padding: var(--space-2);
    border-block-end: 1px solid var(--outline-variant);
    text-align: start;
}

.jobs-table th {
    color: var(--on-surface-variant);
    font-weight: 600;
    white-space: nowrap;
}

.jobs-table .right-align {
    text-align: end;
}

th.sortable {
    cursor: pointer;
    user-select: none;
}

.nowrap {
    white-space: nowrap;
}

.chart-loading {
    display: grid;
    place-items: center;
    block-size: 160px;
}

.card-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-4);
}

.card-head > h2 {
    flex: 1 1 auto;
}

.card-head > :not(h2) {
    flex: 0 1 16rem;
    min-inline-size: 8rem;
    max-inline-size: 100%;
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
