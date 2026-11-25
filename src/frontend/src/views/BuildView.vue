<template>
    <NotFound
        v-if="empty"
        :title="`Build #${id} isn't here`"
        detail="It doesn't exist, cleanup removed it with the older builds, or you may not see its job."
    />
    <div
        v-else
        class="ui build-page stack gap-4"
    >
        <header class="build-header stack gap-2">
            <div class="cluster between gap-3">
                <div class="cluster nowrap gap-3 grow">
                    <StatusIcon
                        :status="statusUpdate.status || 'pending'"
                        class="build-status"
                    />
                    <div class="stack gap-0 grow">
                        <div class="cluster gap-3">
                            <h1 class="title">{{ statusUpdate.name }}</h1>
                            <JobHistory
                                v-if="statusUpdate.name"
                                :job-name="statusUpdate.name"
                                :build-id="statusUpdate.id"
                                :status="statusUpdate.status"
                                :embedded="embedded"
                            />
                        </div>
                        <p
                            v-if="job.desc"
                            class="muted"
                        >
                            {{ job.desc }}
                        </p>
                    </div>
                </div>
                <div class="cluster nowrap gap-1">
                    <button
                        v-if="canRun && isDone"
                        type="button"
                        class="btn"
                        data-cy="run-again-button"
                        @click="runOpen = true"
                    >
                        <i class="icon small">replay</i> Run again
                    </button>
                    <button
                        v-if="canRun && !isDone"
                        type="button"
                        class="btn"
                        data-cy="abort-build-button"
                        @click="abort"
                    >
                        <i class="icon small">stop</i> Abort
                    </button>
                    <!-- Only while it can still end -->
                    <SubscribeButton
                        v-if="statusUpdate.id && !isDone"
                        :build-id="statusUpdate.id"
                    />
                    <UiMenu label="More build actions">
                        <button
                            v-if="canRun && statusUpdate.status === 'pending'"
                            type="button"
                            role="menuitem"
                            data-cy="start-build-button"
                            @click="startNow"
                        >
                            <i class="icon">play_arrow</i> Start now
                        </button>
                        <button
                            v-if="canRun && isDone"
                            type="button"
                            role="menuitem"
                            @click="runOpen = true"
                        >
                            <i class="icon">tune</i> Run again with changes
                        </button>
                        <button
                            type="button"
                            role="menuitem"
                            @click="hideAllLogs = !hideAllLogs"
                        >
                            <i class="icon">{{ hideAllLogs ? "visibility" : "visibility_off" }}</i>
                            {{ hideAllLogs ? "Show logs" : "Hide logs (Ctrl+Alt+H)" }}
                        </button>
                    </UiMenu>
                </div>
            </div>
            <p
                class="muted text-sm nums"
                data-cy="build-facts"
            >
                <span>#{{ statusUpdate.id }}</span>
                <span v-if="statusText">
                    ·
                    <span
                        data-cy="duration"
                        :class="{ over: overUsual }"
                        >{{ statusText }}</span
                    ></span
                >
                <span v-if="silentFor">
                    ·
                    <span
                        class="over"
                        data-cy="silent"
                        >No output for {{ silentFor }}</span
                    ></span
                >
                <span v-if="startedText">
                    · <span data-cy="started-ago">{{ startedText }}</span></span
                >
            </p>
            <div
                v-if="values.length || allParams.length"
                class="cluster gap-1 text-sm build-values"
            >
                <template
                    v-for="(item, i) in values"
                    :key="item.name"
                >
                    <span
                        v-if="i"
                        class="muted"
                        aria-hidden="true"
                        >·</span
                    >
                    <span
                        class="muted"
                        :data-tip="`${item.name}: ${item.value}`"
                        data-cy="params-value"
                        >{{ item.short }}</span
                    >
                </template>
                <button
                    v-if="allParams.length"
                    type="button"
                    class="btn btn-ghost btn-sm"
                    data-cy="all-params"
                    @click="paramsOpen = true"
                >
                    All {{ allParams.length }} {{ allParams.length === 1 ? "parameter" : "parameters" }}
                </button>
            </div>
        </header>

        <BuildGroup
            v-if="statusUpdate.id"
            :build-id="statusUpdate.id"
            :parent="statusUpdate.parent || 0"
            :done="isDone"
            :embedded="embedded"
        />

        <section
            v-if="summary"
            class="panel stack gap-2"
            data-cy="build-summary"
        >
            <h2>Summary</h2>
            <!-- Plain text: a summary can hold text from logs or pull requests -->
            <pre class="summary">{{ summary }}</pre>
        </section>

        <section
            v-if="failedTask && !hideAllLogs"
            ref="failure"
            class="stack gap-2"
            data-cy="task-failed"
        >
            <h2 class="failure-title">
                <span class="failed-name">{{ failedTask.name }}</span>
                {{ failedTask.status === "timed out" ? "timed out" : "failed" }}
                <span class="muted text-sm nums">{{ taskDuration(failedTask) }}</span>
            </h2>
            <LogViewer
                :build-id="id"
                :task="failedTask"
                open-at-end
                :mark="markFor(failedTask)"
                @mark="setMark(failedTask, $event)"
            />
        </section>

        <form
            v-if="!hideAllLogs"
            class="cluster nowrap gap-1"
            @submit.prevent="searchAll"
        >
            <label class="input-icon grow">
                <i class="icon small">manage_search</i>
                <input
                    v-model="allQuery"
                    type="search"
                    class="input"
                    aria-label="Search all tasks"
                    placeholder="Search all tasks"
                    data-cy="search-all"
                />
            </label>
            <button
                type="submit"
                class="btn"
                :disabled="allQuery.trim() === ''"
            >
                Search
            </button>
        </form>
        <section
            v-if="allResults"
            class="panel stack gap-3"
            data-cy="search-all-results"
        >
            <div
                v-for="result in allResults"
                :key="result.task.id"
                class="stack gap-1"
            >
                <div class="cluster gap-2">
                    <span class="strong">{{ result.task.name }}</span>
                    <span class="muted text-sm">{{ result.text }}</span>
                </div>
                <button
                    v-for="match in result.matches"
                    :key="match.line"
                    type="button"
                    class="search-hit text-sm"
                    data-cy="search-all-hit"
                    @click="openAt(result.task, match.line)"
                >
                    <span class="muted nums">{{ match.line }}</span>
                    <span class="mono ellipsis">{{ match.text }}</span>
                </button>
            </div>
        </section>

        <section
            class="stack gap-0 task-list"
            data-cy="tasks"
        >
            <template
                v-for="group in taskGroups"
                :key="group.kind"
            >
                <h2
                    v-if="group.title"
                    class="group-title muted text-sm"
                >
                    {{ group.title }}
                </h2>
                <template
                    v-for="task in group.tasks"
                    :key="task.id"
                >
                    <button
                        type="button"
                        class="task-row"
                        data-cy="task-title"
                        :data-cy-task="task.id"
                        :aria-expanded="isOpen(task) ? 'true' : 'false'"
                        @click="toggleTask(task)"
                    >
                        <span
                            v-if="notRun(task)"
                            class="not-run muted"
                            data-cy-status="pending"
                            aria-hidden="true"
                            >–</span
                        >
                        <StatusIcon
                            v-else
                            :status="task.status"
                            small
                        />
                        <span class="task-name ellipsis">{{ task.name }}</span>
                        <span
                            v-if="notRun(task)"
                            class="muted text-sm"
                            >not run</span
                        >
                        <span
                            v-else
                            class="muted text-sm nums"
                            data-cy="duration"
                            >{{ taskDuration(task) }}</span
                        >
                        <i class="icon small muted">{{ isOpen(task) ? "expand_less" : "expand_more" }}</i>
                    </button>
                    <div
                        v-if="isOpen(task)"
                        class="task-log"
                    >
                        <LogViewer
                            :build-id="id"
                            :task="task"
                            :mark="markFor(task)"
                            @mark="setMark(task, $event)"
                        />
                    </div>
                </template>
            </template>
        </section>

        <ArtifactItem
            :artifacts="getArtifacts"
            :build-i-d="statusUpdate.id"
        />

        <UiDialog
            v-model:open="paramsOpen"
            :title="`${allParams.length} ${allParams.length === 1 ? 'parameter' : 'parameters'}`"
        >
            <dl
                class="params-list"
                data-cy="params-list"
            >
                <template
                    v-for="item in allParams"
                    :key="item.name"
                >
                    <dt :class="{ changed: item.changed }">{{ item.name }}</dt>
                    <dd class="mono">{{ item.value === "" ? "—" : item.value }}</dd>
                </template>
            </dl>
            <template #actions>
                <button
                    v-if="canRun"
                    type="button"
                    class="btn"
                    @click="rerunWithChanges"
                >
                    Run again with changes
                </button>
                <button
                    type="button"
                    class="btn btn-primary"
                    @click="paramsOpen = false"
                >
                    Close
                </button>
            </template>
        </UiDialog>

        <RunDialog
            v-if="statusUpdate.name"
            v-model:open="runOpen"
            :job-name="statusUpdate.name"
            :params="statusUpdate.params || []"
            :again="statusUpdate.parent ? statusUpdate.id : 0"
            :suggested="suggested"
            :defaults="jobInfo.defaults"
            :labels="job.labels || []"
            :param-info="job.param_info || {}"
        />
    </div>
</template>

<script>
import { ago, formatDuration, isZeroTime } from "@/analytics.js";
import ArtifactItem from "@/components/ArtifactItem.vue";
import BuildGroup from "@/components/BuildGroup.vue";
import JobHistory from "@/components/JobHistory.vue";
import { showPlain, showStatus } from "@/favicon.js";
import LogViewer from "@/components/LogViewer.vue";
import NotFound from "@/components/NotFound.vue";
import RunDialog from "@/components/RunDialog.vue";
import { suggestedValues } from "@/summary.js";
import StatusIcon from "@/components/StatusIcon.vue";
import SubscribeButton from "@/components/SubscribeButton.vue";
import UiDialog from "@/components/ui/UiDialog.vue";
import UiMenu from "@/components/ui/UiMenu.vue";
import { ordinal, rowValues } from "@/feed.js";
import axios from "axios";
import vuex from "vuex";

// Where the build's hooks go in the task list, in the order they run
const GROUPS = [
    { kind: "pending", title: "Before start" },
    { kind: "running", title: "At start" },
    { kind: "main", title: "" },
    { kind: "failed", title: "After failure" },
    { kind: "aborted", title: "After abort" },
    { kind: "finished", title: "After success" },
    { kind: "finally", title: "Always" },
];

const DONE = ["failed", "finished", "aborted", "timed out", "skipped"];
// A running build this many times its usual duration is marked, and one
// past it that printed nothing for this long is flagged (L3)
const OVER_USUAL = 1.5;
const SILENT_MS = 2 * 60 * 1000;

// The build page: one header, then the failure, then a plain task list
// (B1-B3 in docs/ui-roadmap.md). Logs open under their task.
export default {
    components: { ArtifactItem, BuildGroup, JobHistory, LogViewer, NotFound, RunDialog, StatusIcon, SubscribeButton, UiDialog, UiMenu },
    props: {
        id: {
            type: Number,
            required: true,
        },
        // Shown beside the feed (F9): the page's title stays the feed's
        embedded: {
            type: Boolean,
            default: false,
        },
    },
    data: function () {
        return {
            job: {},
            summary: "",
            summaryRequests: 0,
            statusUpdate: {
                tasks: [],
                id: NaN,
            },
            // Each task's name, log and log size, which only the build's
            // fetch has
            taskInfo: {},
            buildLogSubscription: "build:log:" + this.id,
            buildUpdateSubscription: "build:update:" + this.id,
            hideAllLogs: false,
            empty: false,
            // The tasks whose log is open, by ID
            open: {},
            // Lines of a log in the address: { task, from, to }
            mark: null,
            runOpen: false,
            paramsOpen: false,
            allQuery: "",
            // Per task: { task, text, matches: [{ line, text }] }, null
            // until searched
            allResults: null,
            now: Date.now(),
            // When the build last printed a line, as far as the page saw
            lastOutputAt: Date.now(),
        };
    },
    computed: {
        ...vuex.mapState(["ws", "queue"]),
        ...vuex.mapGetters(["can"]),
        canRun() {
            return this.can("run", this.statusUpdate.name);
        },
        // What the summary suggests for Run again, e.g. only the failed tests
        suggested() {
            return suggestedValues(
                this.summary,
                (this.statusUpdate.params || []).map((param) => Object.keys(param)[0]),
            );
        },
        isDone() {
            return DONE.includes(this.statusUpdate.status);
        },
        tasks() {
            return (this.statusUpdate.tasks || []).map((task) => ({
                ...this.taskInfo[task.id],
                ...task,
                name: this.job.tasks?.[task.id]?.name || this.taskInfo[task.id]?.name || `Task ${task.id}`,
            }));
        },
        // Main and finally tasks always show; a hook only once it ran
        taskGroups() {
            const visible = this.tasks.filter((task) => task.kind === "main" || task.kind === "finally" || !isZeroTime(task.startedAt));
            return GROUPS.map((group) => ({ ...group, tasks: visible.filter((task) => task.kind === group.kind) })).filter((group) => group.tasks.length);
        },
        // The main task that stopped the build, whose log opens first (B2)
        failedTask() {
            if (!["failed", "timed out"].includes(this.statusUpdate.status)) {
                return null;
            }
            return this.tasks.find((task) => task.kind === "main" && ["failed", "timed out"].includes(task.status)) || null;
        },
        started() {
            return !isZeroTime(this.statusUpdate.startedAt);
        },
        elapsedNs() {
            return Math.max(0, this.now - new Date(this.statusUpdate.startedAt).getTime()) * 1e6;
        },
        // Far over its usual duration (L3)
        overUsual() {
            return this.statusUpdate.status === "running" && this.started && this.statusUpdate.eta > 0 && this.elapsedNs > OVER_USUAL * this.statusUpdate.eta;
        },
        // Past its usual duration and quiet for a while, which a hung
        // command looks like; known only while the logs stream
        silentFor() {
            if (
                this.statusUpdate.status !== "running" ||
                !this.started ||
                !this.statusUpdate.eta ||
                this.hideAllLogs ||
                this.elapsedNs <= this.statusUpdate.eta
            ) {
                return "";
            }
            const quiet = this.now - this.lastOutputAt;
            return quiet >= SILENT_MS ? formatDuration(quiet * 1e6) : "";
        },
        // "Failed after 7m 46s", "Running for 2m 03s", "3rd in queue"
        statusText() {
            const status = this.statusUpdate.status;
            if (status === "pending") {
                const i = this.queue.queuedIDs.indexOf(this.statusUpdate.id);
                return i < 0 ? "Queued" : `${ordinal(i + 1)} in queue`;
            }
            if (status === "running" && this.started) {
                const running = `Running for ${formatDuration(this.elapsedNs)}`;
                if (!this.statusUpdate.eta) {
                    return running;
                }
                const usual = formatDuration(this.statusUpdate.eta);
                return this.overUsual ? `${running}, usually about ${usual}` : `${running} of about ${usual}`;
            }
            if (!this.statusUpdate.duration) {
                return status ? status[0].toUpperCase() + status.slice(1) : "";
            }
            const took = formatDuration(this.statusUpdate.duration);
            return (
                { finished: `Finished in ${took}`, failed: `Failed after ${took}`, aborted: `Aborted after ${took}`, "timed out": `Timed out after ${took}` }[
                    status
                ] || took
            );
        },
        // "started 11 min ago, 11:20 UTC"
        startedText() {
            if (!this.started) {
                return "";
            }
            const at = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", timeZoneName: "short" }).format(
                new Date(this.statusUpdate.startedAt),
            );
            return `started ${this.now && ago(this.statusUpdate.startedAt)}, ${at}`;
        },
        jobInfo() {
            return { defaults: Object.assign({}, ...(this.job.defaultParams || [])), labels: this.job.labels || [] };
        },
        values() {
            return rowValues(this.statusUpdate.params, this.jobInfo).slice(0, 6);
        },
        // Every parameter, the changed ones first (B6)
        allParams() {
            const defaults = this.jobInfo.defaults;
            const params = (this.statusUpdate.params || []).map((param) => {
                const [name, value] = Object.entries(param)[0];
                return { name, value, changed: !(Object.hasOwn(defaults, name) && defaults[name] === value) };
            });
            return [...params.filter((p) => p.changed), ...params.filter((p) => !p.changed)];
        },
        getArtifacts() {
            if (this.statusUpdate.build_artifacts) {
                return this.statusUpdate.build_artifacts;
            }
            return (this.statusUpdate.artifacts || []).map((filename) => ({ filename }));
        },
    },
    watch: {
        "ws.connected": "onWSChange",
        "ws.reconnects": "fetch",
        hideAllLogs: "onHideAllLogsChange",
        // A running task opens in its place and follows its end (B2)
        tasks(tasks) {
            for (const task of tasks) {
                if (task.status === "running" && !(task.id in this.open)) {
                    this.open = { ...this.open, [task.id]: true };
                }
            }
        },
    },
    mounted() {
        if (!this.embedded) {
            this.$store.commit("SET_CURRENT_PAGE", `#${this.id}`);
        }
        document.addEventListener("keyup", this.onKeyUp);
        this.mark = this.parseMark(this.$route.hash);
        if (this.mark) {
            this.open = { [this.mark.task]: true };
        }
        this.fetch();
        this.subscribe();
        this.emitter.on(this.buildUpdateSubscription, this.applyBuildUpdate);
        this.emitter.on("*", this.onAnyEvent);
        this.clock = setInterval(() => (this.now = Date.now()), 1000);
    },
    unmounted() {
        document.removeEventListener("keyup", this.onKeyUp);
        this.unsubscribe();
        this.emitter.off(this.buildUpdateSubscription, this.applyBuildUpdate);
        this.emitter.off("*", this.onAnyEvent);
        clearInterval(this.clock);
        if (!this.embedded) {
            showPlain();
        }
    },
    methods: {
        subscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:subscribe",
                data: { to: this.hideAllLogs ? [this.buildUpdateSubscription] : [this.buildLogSubscription, this.buildUpdateSubscription] },
            });
        },
        unsubscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:unsubscribe",
                data: { to: [this.buildLogSubscription, this.buildUpdateSubscription] },
            });
        },
        fetch() {
            const request = ++this.summaryRequests;
            axios
                .get(`/api/build/${this.id}`)
                .then((response) => {
                    this.statusUpdate = response.data.status_update;
                    this.job = response.data.job;
                    this.taskInfo = Object.fromEntries(
                        (response.data.status_update.tasks || []).map((task) => [task.id, { name: task.name, log: task.log, logSize: task.logSize }]),
                    );
                    if (request === this.summaryRequests) {
                        this.summary = response.data.summary || "";
                    }
                    this.updateTitle();
                })
                .catch(() => {
                    this.empty = true;
                });
        },
        fetchSummary() {
            const request = ++this.summaryRequests;
            axios
                .get(`/api/build/${this.id}`)
                .then((response) => {
                    if (request === this.summaryRequests) {
                        this.summary = response.data.summary || "";
                    }
                })
                .catch(() => {});
        },
        abort() {
            axios
                .post(`/api/build/${this.id}/abort`)
                .then(() => this.$notify({ text: `#${this.id} has been aborted`, type: "primary" }))
                .catch(() => {});
        },
        startNow() {
            axios
                .post(`/api/build/${this.id}/start`)
                .then(() => this.$notify({ text: `#${this.id} has been started`, type: "primary" }))
                .catch(() => {});
        },
        applyBuildUpdate(ev) {
            this.statusUpdate = Object.assign({}, this.statusUpdate, ev);
            this.updateTitle();
            // The last tasks usually write the summary, which updates don't
            // carry. The status is final before the on_* and finally tasks
            // run, so each of their updates reads it again.
            if (this.isDone) {
                this.fetchSummary();
            }
        },
        updateTitle() {
            if (this.embedded) {
                return;
            }
            const icons = { failed: "❌", finished: "✅", aborted: "🛑", "timed out": "⏱️", skipped: "⤵️", pending: "⏳", running: "▶️" };
            this.$store.commit("SET_CURRENT_PAGE", { title: `#${this.id} - ${this.statusUpdate.status}`, icon: icons[this.statusUpdate.status] || "" });
            if (this.statusUpdate.status) {
                showStatus(this.statusUpdate.status);
            }
        },
        taskDuration(task) {
            if (task.status === "running" && !isZeroTime(task.startedAt)) {
                return formatDuration(Math.max(0, this.now - new Date(task.startedAt).getTime()) * 1e6);
            }
            return task.duration ? formatDuration(task.duration) : "";
        },
        // A main task that never ran, in a build that ended
        notRun(task) {
            return task.kind === "main" && task.status === "pending" && this.isDone;
        },
        isOpen(task) {
            return !this.hideAllLogs && Boolean(this.open[task.id]) && task !== this.failedTask && this.failedTask?.id !== task.id;
        },
        // A click on a task opens its log under it; the failed task's log
        // is above the list already, and its row shows it
        toggleTask(task) {
            if (this.failedTask?.id === task.id && !this.hideAllLogs) {
                this.$refs.failure?.scrollIntoView({ behavior: "smooth", block: "start" });
                return;
            }
            this.open = { ...this.open, [task.id]: !this.open[task.id] };
        },
        parseMark(hash) {
            const m = /^#t(\d+)-L(\d+)(?:-L(\d+))?$/.exec(hash || "");
            if (!m) {
                return null;
            }
            const from = Number(m[2]);
            return { task: Number(m[1]), from, to: Math.max(from, Number(m[3] || m[2])) };
        },
        markFor(task) {
            return this.mark && this.mark.task === task.id ? this.mark : null;
        },
        // setMark puts the lines a user marked in the address, to share them
        setMark(task, lines) {
            this.mark = { task: task.id, ...lines };
            const hash = `#t${task.id}-L${lines.from}${lines.to > lines.from ? `-L${lines.to}` : ""}`;
            this.$router.replace({ query: this.$route.query, hash });
        },
        // searchAll finds a text in every task's log with the server's
        // grep, as the log's own search does: ignoring case and the time
        // prefix. Tasks that haven't started have no log yet.
        searchAll() {
            const query = this.allQuery.trim();
            const tasks = this.taskGroups.flatMap((group) => group.tasks);
            Promise.all(
                tasks.map((task) => {
                    if (isZeroTime(task.startedAt)) {
                        return Promise.resolve({ task, text: "not started", matches: [] });
                    }
                    return axios
                        .get(`/api/build/${this.id}/log/${task.id}`, { params: { grep: query }, responseType: "text", transformResponse: (data) => data })
                        .then((response) => this.grepResult(task, response.data))
                        .catch(() => ({ task, text: "no log", matches: [] }));
                }),
            ).then((results) => (this.allResults = results));
        },
        // grepResult reads grep's "N: line" answer, which ends with how many
        // more lines it left out past 1,000
        grepResult(task, text) {
            const lines = text === "" ? [] : text.split("\n");
            let more = 0;
            const matches = [];
            for (const line of lines) {
                const left = /^\.\.\. (\d+) more matching lines$/.exec(line);
                if (left) {
                    more = Number(left[1]);
                    continue;
                }
                const m = /^(\d+): (.*)$/.exec(line);
                if (m) {
                    matches.push({ line: Number(m[1]), text: m[2] });
                }
            }
            const count = matches.length + more;
            let summary = count === 0 ? "no matches" : `${more ? "1,000+" : count} ${count === 1 ? "match" : "matches"}`;
            if (task.status === "running") {
                summary += ", more as it runs";
            }
            return { task, text: summary, matches: matches.slice(0, 20) };
        },
        // openAt opens a task's log at a line
        openAt(task, line) {
            if (this.failedTask?.id !== task.id) {
                this.open = { ...this.open, [task.id]: true };
            }
            this.setMark(task, { from: line, to: line });
        },
        rerunWithChanges() {
            this.paramsOpen = false;
            this.runOpen = true;
        },
        onWSChange(value) {
            if (value) {
                this.subscribe();
            } else {
                this.unsubscribe();
            }
        },
        // A log line of this build's tasks
        onAnyEvent(type) {
            if (type.startsWith(`${this.buildLogSubscription}:task-`)) {
                this.lastOutputAt = Date.now();
            }
        },
        onKeyUp(event) {
            if (event.ctrlKey && event.altKey && event.code === "KeyH") {
                this.hideAllLogs = !this.hideAllLogs;
            }
        },
        // Hidden logs don't stream; shown again, each opens fresh
        onHideAllLogsChange(value) {
            this.$store.commit("WS_SEND", { type: value ? "in:unsubscribe" : "in:subscribe", data: { to: [this.buildLogSubscription] } });
        },
    },
};
</script>

<style scoped>
.over {
    color: var(--warning);
    font-weight: 600;
}

.build-status {
    inline-size: 28px;
    block-size: 28px;
}

.build-status :deep(.icon) {
    font-size: 28px;
}

.failure-title {
    font-size: var(--text-md);
}

.failed-name {
    color: var(--status-failed);
}

.summary {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
}

.task-list {
    border-block-start: 1px solid var(--outline-variant);
}

.group-title {
    padding: var(--space-3) var(--space-2) var(--space-1);
    font-weight: 600;
}

.task-row {
    display: grid;
    grid-template-columns: 16px minmax(0, 1fr) auto auto;
    align-items: center;
    gap: var(--space-3);
    min-block-size: 34px;
    inline-size: 100%;
    padding: var(--space-1) var(--space-2);
    border: none;
    border-block-end: 1px solid var(--outline-variant);
    background: none;
    color: var(--on-surface);
    font: inherit;
    text-align: start;
    cursor: pointer;
}

.task-row:hover {
    background-color: color-mix(in srgb, var(--on-surface) 4%, transparent);
}

.not-run {
    text-align: center;
}

.task-log {
    padding: var(--space-2) 0 var(--space-3);
    border-block-end: 1px solid var(--outline-variant);
}

.search-hit {
    display: grid;
    grid-template-columns: 3.5rem minmax(0, 1fr);
    gap: var(--space-2);
    padding: 2px var(--space-1);
    border: none;
    border-radius: 4px;
    background: none;
    color: var(--on-surface);
    font: inherit;
    font-size: var(--text-sm);
    text-align: start;
    cursor: pointer;
}

.search-hit:hover {
    background-color: color-mix(in srgb, var(--on-surface) 6%, transparent);
}

.search-hit .nums {
    text-align: end;
}

.params-list {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: var(--space-1) var(--space-4);
}

.params-list dt {
    color: var(--on-surface-variant);
}

.params-list dt.changed {
    color: var(--on-surface);
    font-weight: 600;
}

.params-list dd {
    margin: 0;
    overflow-wrap: anywhere;
}
</style>
