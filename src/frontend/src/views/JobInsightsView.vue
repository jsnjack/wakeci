<template>
    <div class="insights-page">
        <div class="tabs left-align">
            <router-link
                :to="{ name: 'jobEdit', params: { name } }"
                data-cy="job-file-tab"
            >
                <i>description</i>
                <span>{{ canAny("write") ? "Edit" : "View" }}</span>
            </router-link>
            <a
                class="active"
                data-cy="job-insights-tab"
            >
                <i>insights</i>
                <span>Insights</span>
            </a>
        </div>

        <nav class="wrap insight-filters">
            <nav class="group connected">
                <button
                    v-for="(count, i) in counts"
                    :key="count"
                    :class="{
                        active: builds === count,
                        'left-round': i === 0,
                        'right-round': i === counts.length - 1,
                        'no-round': i > 0 && i < counts.length - 1,
                    }"
                    :data-cy="`builds-${count}`"
                    @click.prevent="builds = count"
                >
                    Last {{ count }}
                </button>
            </nav>
            <progress
                v-show="loading"
                class="circle small"
            ></progress>
            <small
                v-if="data"
                class="muted"
                data-cy="in-history"
            >
                {{ data.inHistory }} {{ data.inHistory === 1 ? "build" : "builds" }} of {{ name }} in the history, which keeps the last builds of all jobs
                (Settings)
            </small>
        </nav>

        <template v-if="data && data.builds.length">
            <article
                v-if="data.failing"
                class="border no-elevate padding row failing"
                data-cy="insights-failing"
            >
                <BuildStatus :status="data.failing.last.status" />
                <div class="max">
                    Failing since #{{ data.failing.first.id }}, {{ ago(data.failing.first.startedAt) }}: {{ data.failing.builds }}
                    {{ data.failing.builds === 1 ? "build" : "builds in a row"
                    }}<span v-if="data.failing.last.failedTask"
                        >, the last stopped at <b>{{ data.failing.last.failedTask }}</b></span
                    >.
                </div>
                <router-link
                    :to="{ name: 'build', params: { id: data.failing.last.id } }"
                    class="button border small"
                    >Open build</router-link
                >
            </article>

            <div class="grid">
                <article
                    v-for="tile in tiles"
                    :key="tile.key"
                    class="border no-elevate padding s6 l3"
                    :data-cy="`insight-${tile.key}`"
                >
                    <div class="muted tile-label">{{ tile.label }}</div>
                    <div
                        class="tile-value"
                        data-cy="tile-value"
                    >
                        {{ tile.value }}
                    </div>
                    <div class="muted tile-sub">{{ tile.sub }}</div>
                </article>
            </div>

            <article class="border no-elevate padding chart-card">
                <h6 class="small no-margin">Duration of each build, by task</h6>
                <TaskBarsChart
                    :builds="data.builds"
                    :series-of="seriesOf"
                />
                <div class="chart-legend">
                    <span
                        v-for="item in taskLegend"
                        :key="item.name"
                    >
                        <span
                            class="swatch"
                            :class="item.cls"
                        ></span
                        >{{ item.name }}
                    </span>
                </div>
                <StatusLegend />
                <p class="chart-note">Oldest on the left. The mark under a bar is the build's result. Click a bar to open the build.</p>
            </article>

            <article
                class="border no-elevate padding"
                data-cy="tasks-table"
            >
                <h6 class="small no-margin">Tasks</h6>
                <div class="table-scroll">
                    <table class="small-text">
                        <thead>
                            <tr>
                                <th>Task</th>
                                <th class="right-align">Runs</th>
                                <th class="right-align">Failed</th>
                                <th class="right-align">Skipped</th>
                                <th class="right-align">Median</th>
                                <th class="right-align">p95</th>
                                <th class="right-align">Last 10 runs</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr
                                v-for="task in data.tasks"
                                :key="task.name"
                                :data-cy="`task-row-${task.name}`"
                            >
                                <td class="nowrap">
                                    <span
                                        class="legend-swatch"
                                        :class="seriesOf(task.name)"
                                    ></span
                                    >{{ task.name }}
                                </td>
                                <td class="right-align">{{ task.runs }}</td>
                                <td
                                    class="right-align"
                                    data-cy="task-failed"
                                >
                                    {{ task.failed }}
                                </td>
                                <td class="right-align">{{ task.skipped }}</td>
                                <td class="right-align">{{ task.duration.count ? formatDuration(task.duration.p50) : "—" }}</td>
                                <td class="right-align">{{ task.duration.count ? formatDuration(task.duration.p95) : "—" }}</td>
                                <td
                                    class="right-align nowrap"
                                    :title="task.change === null ? 'Needs 15 finished runs' : 'Median of the last 10 finished runs against the runs before'"
                                >
                                    {{ changeText(task.change) }}
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </article>

            <div class="grid">
                <article
                    class="border no-elevate padding s12 l6"
                    data-cy="stopped-at"
                >
                    <h6 class="small no-margin">Where failed builds stopped</h6>
                    <div
                        v-for="item in data.stoppedAt"
                        :key="item.task"
                        class="bar-row"
                    >
                        <span class="ellipsis">{{ item.task }}</span>
                        <span class="bar"><span :style="{ width: percent(item.builds / failedBuilds) }"></span></span>
                        <span class="muted right-align">{{ item.builds }} of {{ failedBuilds }}</span>
                    </div>
                    <p
                        v-if="!data.stoppedAt.length"
                        class="chart-note"
                    >
                        None of these builds failed.
                    </p>
                </article>
                <article
                    class="border no-elevate padding s12 l6"
                    data-cy="params"
                >
                    <h6 class="small no-margin">Success by parameter</h6>
                    <template v-if="data.params.length">
                        <div
                            v-if="data.params.length > 1"
                            class="field label suffix border small round"
                        >
                            <select v-model="paramName">
                                <option
                                    v-for="p in data.params"
                                    :key="p.name"
                                    :value="p.name"
                                >
                                    {{ p.name }}
                                </option>
                            </select>
                            <label>Parameter</label>
                            <i>arrow_drop_down</i>
                        </div>
                        <table class="small-text">
                            <thead>
                                <tr>
                                    <th>{{ param.name }}</th>
                                    <th class="right-align">Builds</th>
                                    <th class="right-align">Success</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr
                                    v-for="value in param.values"
                                    :key="value.value"
                                >
                                    <td class="ellipsis param-value">
                                        <code>{{ value.value || "(empty)" }}</code>
                                    </td>
                                    <td class="right-align">{{ value.builds }}</td>
                                    <td class="right-align">{{ formatRate(value.successRate) }}</td>
                                </tr>
                            </tbody>
                        </table>
                    </template>
                    <p
                        v-else
                        class="chart-note"
                    >
                        No parameter had more than one value in these builds.
                    </p>
                </article>
            </div>
        </template>

        <article
            v-else-if="data"
            class="border no-elevate medium-height middle-align center-align"
            data-cy="insights-empty"
        >
            <div class="center-align">
                <i class="extra">insights</i>
                <h5>No builds of {{ name }} in the history</h5>
                <p class="muted">Insights appear once a build of this job has ended.</p>
            </div>
        </article>
    </div>
</template>

<script>
import "@/assets/charts.css";
import { STATUSES, ago, formatDuration, formatRate } from "@/analytics";
import BuildStatus from "@/components/BuildStatus.vue";
import StatusLegend from "@/components/charts/StatusLegend.vue";
import TaskBarsChart from "@/components/charts/TaskBarsChart.vue";
import axios from "axios";
import debounce from "lodash/debounce";
import vuex from "vuex";

const COUNTS = [20, 40, 100];
// Main tasks with a colour of their own; the others share one
const TASK_SERIES = 5;
const FINAL = new Set(STATUSES.map((s) => s.status));

export default {
    components: { BuildStatus, StatusLegend, TaskBarsChart },
    props: {
        name: {
            type: String,
            required: true,
        },
    },
    data() {
        return {
            counts: COUNTS,
            builds: 40,
            data: null,
            loading: false,
            paramName: "",
            subscription: "build:update:",
        };
    },
    computed: {
        ...vuex.mapState(["ws"]),
        ...vuex.mapGetters(["canAny"]),
        tiles() {
            const d = this.data;
            return [
                {
                    key: "success",
                    label: "Success rate",
                    value: formatRate(d.totals.successRate),
                    sub: `${d.totals.failed + d.totals.timedOut} of ${d.totals.builds} failed or timed out`,
                },
                {
                    key: "flaky",
                    label: "Flaky failures",
                    value: d.flakyChecked ? `${d.flaky} of ${d.flakyChecked}` : "—",
                    sub: "failed, then passed with the same parameters",
                },
                { key: "fix", label: "Time to fix", ...this.timeToFix },
                {
                    key: "duration",
                    label: "Median duration",
                    value: d.duration.count ? formatDuration(d.duration.p50) : "—",
                    sub: d.duration.count ? `p95 ${formatDuration(d.duration.p95)} · finished builds` : "no finished builds",
                },
            ];
        },
        timeToFix() {
            const d = this.data;
            const fixes = d.timeToFix.count;
            const since = d.failing ? `since #${d.failing.first.id}, ${ago(d.failing.first.startedAt)}` : "";
            if (fixes) {
                const median = `median of ${fixes} ${fixes === 1 ? "fix" : "fixes"}, from a failure to the next success`;
                return { value: formatDuration(d.timeToFix.p50), sub: d.failing ? `${median}; still failing ${since}` : median };
            }
            if (d.failing) {
                return { value: "Still failing", sub: since };
            }
            return { value: "—", sub: "No failures in these builds" };
        },
        series() {
            const series = {};
            (this.data?.tasks || []).forEach((task, i) => {
                series[task.name] = i < TASK_SERIES ? `series-${i + 1}` : "series-other";
            });
            return series;
        },
        taskLegend() {
            const items = this.data.tasks.slice(0, TASK_SERIES).map((task) => ({ name: task.name, cls: this.series[task.name] }));
            if (this.data.tasks.length > TASK_SERIES) {
                items.push({ name: "other tasks", cls: "series-other" });
            }
            if (this.data.builds.some((b) => b.tasks.some((t) => t.kind !== "main" && t.duration > 0))) {
                items.push({ name: "on_* and finally tasks", cls: "series-hooks" });
            }
            return items;
        },
        failedBuilds() {
            return this.data.totals.failed + this.data.totals.timedOut;
        },
        param() {
            return this.data.params.find((p) => p.name === this.paramName) || this.data.params[0];
        },
    },
    watch: {
        builds: "fetch",
        name() {
            this.data = null;
            this.setPage();
            this.fetch();
        },
        "ws.connected"(connected) {
            if (connected) {
                this.subscribe();
            }
        },
        "ws.reconnects"() {
            this.fetch();
        },
    },
    created() {
        this.refresh = debounce(() => this.fetch(), 3000, { maxWait: 15000 });
    },
    mounted() {
        this.setPage();
        const builds = Number(this.$route.query.builds);
        if (COUNTS.includes(builds)) {
            this.builds = builds;
        }
        this.fetch();
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
        formatDuration,
        formatRate,
        setPage() {
            this.$store.commit("SET_CURRENT_PAGE", `Insights: ${this.name}`);
        },
        fetch() {
            this.loading = true;
            const asked = `${this.name}/${this.builds}`;
            this.asked = asked;
            axios
                .get(`/api/job/${encodeURIComponent(this.name)}/insights`, { params: { builds: this.builds } })
                .then((response) => {
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
        subscribe() {
            this.$store.commit("WS_SEND", { type: "in:subscribe", data: { to: [this.subscription] } });
        },
        onBuildUpdate(build) {
            if (build.name === this.name && FINAL.has(build.status) && build.duration > 0) {
                this.refresh();
            }
        },
        seriesOf(name) {
            return this.series[name] || "series-other";
        },
        percent(share) {
            return `${Math.round((share || 0) * 100)}%`;
        },
        changeText(change) {
            if (change === null) {
                return "—";
            }
            const pct = Math.round(change * 100);
            if (pct === 0) {
                return "no change";
            }
            return `${pct > 0 ? "+" : "−"}${Math.abs(pct)}% ${pct > 0 ? "slower" : "faster"}`;
        },
    },
};
</script>

<style scoped>
.insight-filters {
    gap: 0.75rem;
    align-items: center;
    margin: 0;
}
.insight-filters small {
    flex: 1 1 14rem;
    min-width: 0;
    white-space: normal;
}
.failing {
    gap: 0.75rem;
    align-items: center;
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
}
.legend-swatch {
    display: inline-block;
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 0.2rem;
    margin-right: 0.4rem;
    vertical-align: -0.05rem;
}
.bar-row {
    display: grid;
    grid-template-columns: minmax(6rem, 12rem) 1fr 5rem;
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
.table-scroll {
    overflow-x: auto;
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
.param-value {
    max-width: 16rem;
}
.insights-page {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}
.insights-page > .grid {
    margin: 0;
}
</style>
