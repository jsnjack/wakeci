<template>
    <div class="ui insights-page stack gap-4">
        <JobTabs
            :name="name"
            current="insights"
        />

        <div class="cluster gap-3 insight-filters">
            <div
                class="segmented"
                role="group"
                aria-label="Builds"
            >
                <button
                    v-for="count in counts"
                    :key="count"
                    type="button"
                    :aria-pressed="builds === count ? 'true' : 'false'"
                    :data-cy="`builds-${count}`"
                    @click="builds = count"
                >
                    Last {{ count }}
                </button>
            </div>
            <span
                v-show="loading"
                class="status-ring spinner"
                aria-label="Loading"
            ></span>
            <small
                v-if="data"
                class="muted"
                data-cy="in-history"
            >
                {{ data.inHistory }} {{ data.inHistory === 1 ? "build" : "builds" }} of {{ name }} in the history, which keeps the last builds of all jobs
                (Settings)
            </small>
        </div>

        <template v-if="data && data.builds.length">
            <article
                v-if="data.failing"
                class="panel cluster gap-3 failing"
                data-cy="insights-failing"
            >
                <StatusIcon :status="data.failing.last.status" />
                <div class="grow">
                    Failing since #{{ data.failing.first.id }}, {{ ago(data.failing.first.startedAt) }}: {{ data.failing.builds }}
                    {{ data.failing.builds === 1 ? "build" : "builds in a row"
                    }}<span v-if="data.failing.last.failedTask"
                        >, the last stopped at <b>{{ data.failing.last.failedTask }}</b></span
                    >.
                </div>
                <router-link
                    :to="{ name: 'build', params: { id: data.failing.last.id } }"
                    class="btn btn-sm btn-outline btn-ghost"
                    >Open build</router-link
                >
            </article>

            <div class="cols tiles">
                <article
                    v-for="tile in tiles"
                    :key="tile.key"
                    class="panel"
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

            <article class="panel chart-card">
                <h2 class="card-title">Duration of each build, by task</h2>
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
                class="panel"
                data-cy="tasks-table"
            >
                <h2 class="card-title">Tasks</h2>
                <div class="table-scroll">
                    <table class="data-table text-sm">
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

            <div class="cols chart-grid">
                <article
                    class="panel"
                    data-cy="stopped-at"
                >
                    <h2 class="card-title">Where failed builds stopped</h2>
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
                    class="panel"
                    data-cy="params"
                >
                    <h2 class="card-title">Success by parameter</h2>
                    <template v-if="data.params.length">
                        <label
                            v-if="data.params.length > 1"
                            class="form-field param-pick"
                        >
                            <span class="label-text">Parameter</span>
                            <select
                                v-model="shownParam"
                                class="select"
                            >
                                <option
                                    v-for="p in data.params"
                                    :key="p.name"
                                    :value="p.name"
                                >
                                    {{ p.name }}
                                </option>
                            </select>
                        </label>
                        <div class="table-scroll">
                            <table class="data-table text-sm">
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
                        </div>
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
            class="panel empty"
            data-cy="insights-empty"
        >
            <div class="stack gap-2 empty-body">
                <i class="icon large">insights</i>
                <h2>No builds of {{ name }} in the history</h2>
                <p class="muted">Insights appear once a build of this job has ended.</p>
            </div>
        </article>
    </div>
</template>

<script>
import "@/assets/charts.css";
import { STATUSES, ago, formatDuration, formatRate } from "@/analytics";
import JobTabs from "@/components/JobTabs.vue";
import StatusIcon from "@/components/StatusIcon.vue";
import StatusLegend from "@/components/charts/StatusLegend.vue";
import TaskBarsChart from "@/components/charts/TaskBarsChart.vue";
import { humanFileSize } from "@/store/utils";
import axios from "axios";
import debounce from "lodash/debounce";
import vuex from "vuex";

const COUNTS = [20, 40, 100];
// Main tasks with a colour of their own; the others share one
const TASK_SERIES = 5;
const FINAL = new Set(STATUSES.map((s) => s.status));

export default {
    components: { JobTabs, StatusIcon, StatusLegend, TaskBarsChart },
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
                    sub: d.duration.count ? `p95 ${formatDuration(d.duration.p95)} · successful builds` : "no successful builds",
                },
                { key: "disk", label: "Disk", ...this.disk },
            ];
        },
        // What all the job's builds in the history keep on disk
        disk() {
            const disk = this.data.disk;
            if (!disk?.builds) {
                return { value: "—", sub: "measured as builds end" };
            }
            const parts = [
                ["artifacts", disk.artifacts],
                ["logs", disk.logs],
                ["workspaces", disk.workspace],
            ]
                .filter(([, size]) => size > 0)
                .map(([what, size]) => `${what} ${humanFileSize(size)}`);
            const builds = `${disk.builds} ${disk.builds === 1 ? "build" : "builds"}`;
            const pending = disk.unmeasured ? `, ${disk.unmeasured} not measured yet` : "";
            return {
                value: humanFileSize(disk.logs + disk.artifacts + disk.workspace),
                sub: `${parts.join(" · ") || "empty"} · ${builds}${pending}`,
            };
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
        // The select shows the parameter the table shows, the first until
        // the user picks one
        shownParam: {
            get() {
                return this.param?.name || "";
            },
            set(name) {
                this.paramName = name;
            },
        },
    },
    watch: {
        builds: "fetch",
        name() {
            this.data = null;
            this.setPage();
            this.fetch();
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
        // Counted, since the notifications and the feed use the same topic; a
        // new connection subscribes again by itself
        this.$store.commit("SUBSCRIBE_TOPIC", this.subscription);
        this.emitter.on(this.subscription, this.onBuildUpdate);
    },
    unmounted() {
        this.refresh.cancel();
        this.emitter.off(this.subscription, this.onBuildUpdate);
        this.$store.commit("UNSUBSCRIBE_TOPIC", this.subscription);
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
.insight-filters small {
    flex: 1 1 14rem;
    min-inline-size: 0;
}

.spinner {
    inline-size: 18px;
    block-size: 18px;
    --_c: var(--primary);
}

.panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-inline-size: 0;
}

.panel.failing {
    flex-direction: row;
    flex-wrap: nowrap;
    align-items: center;
}

/* The tiles share the row, however many fit */
.tiles {
    grid-template-columns: repeat(auto-fit, minmax(min(12rem, 100%), 1fr));
}

.chart-grid {
    --min: 26rem;
}

.card-title {
    font-size: var(--text-md);
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
}

.legend-swatch {
    display: inline-block;
    inline-size: 0.7rem;
    block-size: 0.7rem;
    border-radius: 0.2rem;
    margin-inline-end: 0.4rem;
    vertical-align: -0.05rem;
}

.bar-row {
    display: grid;
    grid-template-columns: minmax(6rem, 12rem) 1fr 5rem;
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

.table-scroll {
    overflow-x: auto;
}

.data-table {
    inline-size: 100%;
    border-collapse: collapse;
}

.data-table th,
.data-table td {
    padding: var(--space-2);
    border-block-end: 1px solid var(--outline-variant);
    text-align: start;
}

.data-table th {
    color: var(--on-surface-variant);
    font-weight: 600;
    white-space: nowrap;
}

.data-table .right-align {
    text-align: end;
}

.nowrap {
    white-space: nowrap;
}

.param-value {
    max-inline-size: 16rem;
}

.param-pick {
    max-inline-size: 16rem;
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
