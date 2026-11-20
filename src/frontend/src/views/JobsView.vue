<template>
    <div class="ui jobs-page stack gap-3">
        <div class="cluster gap-2">
            <label class="input-icon grow">
                <i class="icon">filter_alt</i>
                <input
                    type="search"
                    class="input"
                    data-cy="filter"
                    aria-label="Filter jobs by name"
                    placeholder="Filter jobs by name"
                    :value="filter"
                    @input="setFilter($event.target.value)"
                />
            </label>
            <div
                class="segmented"
                role="group"
                aria-label="Sort"
            >
                <button
                    type="button"
                    :aria-pressed="sort === 'name' ? 'true' : 'false'"
                    data-cy="sort-name"
                    @click="sort = 'name'"
                >
                    Name
                </button>
                <button
                    type="button"
                    :aria-pressed="sort === 'failed' ? 'true' : 'false'"
                    data-cy="sort-failed"
                    @click="sort = 'failed'"
                >
                    Recently failed
                </button>
            </div>
            <button
                type="button"
                class="btn btn-outline btn-ghost"
                :aria-pressed="grouped ? 'true' : 'false'"
                data-cy="group-jobs"
                @click="grouped = !grouped"
            >
                <i class="icon small">folder</i> Group
            </button>
            <button
                v-if="canAny('write')"
                type="button"
                class="btn btn-primary"
                data-cy="create-job"
                @click="openCreate"
            >
                <i class="icon small">add</i> Create a job
            </button>
        </div>

        <div
            class="stack gap-4"
            data-cy="jobs-container"
        >
            <section
                v-for="group in groups"
                :key="group.name"
                class="stack gap-0"
            >
                <h2
                    v-if="group.name"
                    class="group-title muted text-sm"
                    data-cy="job-group"
                >
                    {{ group.name }}
                </h2>
                <div class="job-list">
                    <div
                        v-for="job in group.jobs"
                        :key="job.name"
                        class="job-row"
                        :class="{ inactive: job.active !== 'true' }"
                        :data-cy="job.name"
                    >
                        <button
                            type="button"
                            class="btn btn-ghost btn-icon btn-sm tip-start"
                            :aria-pressed="pinned.includes(job.name) ? 'true' : 'false'"
                            :aria-label="pinned.includes(job.name) ? `Unpin ${job.name}` : `Pin ${job.name}`"
                            :data-tip="pinned.includes(job.name) ? 'Unpin' : 'Pin to the top'"
                            data-cy="pin-job"
                            @click="togglePin(job.name)"
                        >
                            <i
                                class="icon small pin"
                                :class="{ on: pinned.includes(job.name) }"
                                >keep</i
                            >
                        </button>
                        <div class="job-main">
                            <router-link
                                :to="{ name: 'jobInsights', params: { name: job.name } }"
                                class="job-name ellipsis"
                                >{{ job.name }}</router-link
                            >
                            <span
                                v-if="job.desc"
                                class="muted text-sm ellipsis"
                                >{{ job.desc }}</span
                            >
                        </div>
                        <OutcomeStrip
                            class="job-strip"
                            :builds="job.recent || []"
                        />
                        <div class="job-meta muted text-sm nums">
                            <span
                                v-if="job.failing"
                                class="failing"
                                data-cy="job-failing"
                                :data-tip="`Failed ${job.failing.builds} ${job.failing.builds === 1 ? 'time' : 'times'} in a row`"
                                >failing since #{{ job.failing.first.id
                                }}{{ job.failing.first.startedAt && !isZero(job.failing.first.startedAt) ? `, ${ago(job.failing.first.startedAt)}` : "" }}</span
                            >
                            <span
                                v-else-if="lastRun(job)"
                                data-cy="job-last-run"
                                >last run {{ ago(lastRun(job)) }}</span
                            >
                            <span
                                v-if="job.nextRun && job.active === 'true'"
                                data-cy="job-next-run"
                                :data-tip="job.interval"
                                >next {{ ago(job.nextRun) }}</span
                            >
                            <span
                                v-else-if="job.interval"
                                :data-tip="'Interval'"
                                class="mono"
                                >{{ job.interval }}</span
                            >
                        </div>
                        <label
                            class="cluster nowrap gap-1 text-sm job-active"
                            :data-tip="job.active === 'true' ? 'On: runs and its interval starts it' : 'Off: doesn\'t run'"
                        >
                            <input
                                type="checkbox"
                                role="switch"
                                :checked="job.active === 'true'"
                                :disabled="!canAny('write')"
                                :aria-label="`${job.name} is on`"
                                data-cy="job-active"
                                @click.prevent="toggleActive(job)"
                            />
                        </label>
                        <div class="cluster nowrap gap-1 job-actions">
                            <button
                                v-if="can('run', job.name)"
                                type="button"
                                class="btn btn-ghost btn-icon"
                                :disabled="job.active !== 'true'"
                                aria-label="Run"
                                data-tip="Run"
                                data-cy="run-job-button"
                                @click="openRun(job)"
                            >
                                <i class="icon">play_arrow</i>
                            </button>
                            <router-link
                                :to="{ name: 'jobInsights', params: { name: job.name } }"
                                class="btn btn-ghost btn-icon"
                                aria-label="Insights"
                                data-tip="Insights"
                                data-cy="job-insights-button"
                            >
                                <i class="icon">insights</i>
                            </router-link>
                            <router-link
                                :to="{ name: 'jobEdit', params: { name: job.name } }"
                                class="btn btn-ghost btn-icon"
                                :aria-label="canAny('write') ? 'Edit' : 'View'"
                                :data-tip="canAny('write') ? 'Edit' : 'View'"
                                data-cy="edit-job-button"
                            >
                                <i class="icon">{{ canAny("write") ? "edit" : "visibility" }}</i>
                            </router-link>
                            <button
                                v-if="canAny('write')"
                                type="button"
                                class="btn btn-ghost btn-icon tip-end"
                                aria-label="Delete"
                                data-tip="Delete"
                                data-cy="delete-job-button"
                                @click="deleting = job.name"
                            >
                                <i class="icon">delete</i>
                            </button>
                        </div>
                    </div>
                </div>
            </section>
            <div
                v-if="shownJobs.length === 0 && fetchingDone"
                class="jobs-empty stack muted"
            >
                <i class="icon large">water</i>
                <span>No jobs found</span>
            </div>
            <div
                v-if="!fetchingDone"
                class="jobs-empty"
            >
                <span class="status-ring jobs-spinner"></span>
            </div>
        </div>

        <RunDialog
            v-if="runJob"
            v-model:open="runOpen"
            :job-name="runJob.name"
            :params="runJob.defaultParams || []"
            :defaults="Object.assign({}, ...(runJob.defaultParams || []))"
            :labels="runJob.labels || []"
            :param-info="runJob.paramInfo || {}"
            remember-last
        />

        <UiDialog
            v-model:open="createOpen"
            title="Create a job"
        >
            <form
                id="create-job-form"
                class="stack gap-2"
                @submit.prevent="create"
            >
                <label class="form-field">
                    <span class="label-text">Name</span>
                    <input
                        ref="newJobInput"
                        v-model="newJobName"
                        class="input"
                        name="new-job-name"
                        required
                    />
                    <span class="hint">Also the job file's name, in the job directory</span>
                </label>
            </form>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="createOpen = false"
                >
                    Cancel
                </button>
                <button
                    type="submit"
                    form="create-job-form"
                    class="btn btn-primary"
                    data-cy="create-job-button"
                >
                    Create
                </button>
            </template>
        </UiDialog>

        <UiDialog
            :open="deleting !== null"
            title="Delete the job?"
            @update:open="deleting = null"
        >
            <p>
                Delete <b>{{ deleting }}</b
                >? Its file goes, and the history keeps its builds.
            </p>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="deleting = null"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    class="btn btn-danger"
                    data-cy="delete-job-confirm"
                    @click="deleteJob"
                >
                    Delete
                </button>
            </template>
        </UiDialog>
    </div>
</template>

<script>
import "@/assets/charts.css";
import { ago, isZeroTime } from "@/analytics.js";
import OutcomeStrip from "@/components/charts/OutcomeStrip.vue";
import RunDialog from "@/components/RunDialog.vue";
import UiDialog from "@/components/ui/UiDialog.vue";
import { escapeHtml } from "@/store/utils.js";
import axios from "axios";
import vuex from "vuex";

const PINNED_KEY = "pinnedJobs";
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

function loadPinned() {
    try {
        const pinned = JSON.parse(window.localStorage.getItem(PINNED_KEY) || "[]");
        return Array.isArray(pinned) ? pinned : [];
    } catch {
        return [];
    }
}

// groupOf is a job's heading: its file's group, or the start of its name up
// to the first - _ . or /
function groupOf(job) {
    if (job.group) {
        return job.group;
    }
    const m = /^(.+?)[-_./]/.exec(job.name);
    return m ? m[1] : job.name;
}

// The jobs page as a status board (J1): each job's last 20 builds, when it
// ran and runs next, and since when it fails; pinned jobs first, sorted by
// name as people expect or by recent failure, grouped on demand
export default {
    components: { OutcomeStrip, RunDialog, UiDialog },
    data() {
        return {
            jobs: [],
            filter: this.$route.query.q || "",
            sort: "name",
            grouped: false,
            pinned: loadPinned(),
            fetchingDone: false,
            runJob: null,
            runOpen: false,
            createOpen: false,
            newJobName: "new_job",
            deleting: null,
        };
    },
    computed: {
        ...vuex.mapGetters(["can", "canAny"]),
        shownJobs() {
            const filter = this.filter.toLowerCase();
            const jobs = this.jobs.filter((job) => job.name.toLowerCase().includes(filter));
            const failedAt = (job) => (job.failing ? new Date(job.failing.last.startedAt).getTime() || job.failing.last.id : -Infinity);
            return jobs.sort((a, b) => {
                const pin = Number(this.pinned.includes(b.name)) - Number(this.pinned.includes(a.name));
                if (pin !== 0) {
                    return pin;
                }
                if (this.sort === "failed") {
                    const failed = failedAt(b) - failedAt(a);
                    if (failed !== 0) {
                        return failed;
                    }
                }
                return collator.compare(a.name, b.name);
            });
        },
        // Pinned jobs stay above the groups
        groups() {
            if (!this.grouped) {
                return [{ name: "", jobs: this.shownJobs }];
            }
            const pinned = this.shownJobs.filter((job) => this.pinned.includes(job.name));
            const byGroup = new Map();
            for (const job of this.shownJobs.filter((j) => !this.pinned.includes(j.name))) {
                const name = groupOf(job);
                byGroup.set(name, [...(byGroup.get(name) || []), job]);
            }
            const groups = [...byGroup.entries()].sort(([a], [b]) => collator.compare(a, b)).map(([name, jobs]) => ({ name, jobs }));
            return pinned.length ? [{ name: "Pinned", jobs: pinned }, ...groups] : groups;
        },
    },
    watch: {
        // Jobs the user may now read; the filter is in the address, so it
        // stays through a reconnect
        "$store.state.ws.reconnects": "fetch",
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Jobs");
        this.fetch();
    },
    methods: {
        ago(value) {
            return ago(value);
        },
        isZero(value) {
            return isZeroTime(value);
        },
        lastRun(job) {
            const last = job.recent?.[job.recent.length - 1];
            return last && !isZeroTime(last.startedAt) ? last.startedAt : null;
        },
        fetch() {
            axios
                .get("/api/jobs/?board=1")
                .then((response) => {
                    this.jobs = response.data || [];
                    this.fetchingDone = true;
                })
                .catch(() => {
                    this.fetchingDone = true;
                });
        },
        setFilter(value) {
            this.filter = value;
            const query = { ...this.$route.query };
            if (value) {
                query.q = value;
            } else {
                delete query.q;
            }
            this.$router.replace({ query });
        },
        togglePin(name) {
            this.pinned = this.pinned.includes(name) ? this.pinned.filter((n) => n !== name) : [...this.pinned, name];
            try {
                window.localStorage.setItem(PINNED_KEY, JSON.stringify(this.pinned));
            } catch {
                // A browser that keeps nothing forgets them on reload
            }
        },
        openRun(job) {
            this.runJob = job;
            this.runOpen = true;
        },
        toggleActive(job) {
            const data = new FormData();
            data.append("active", String(job.active !== "true"));
            axios
                .post(`/api/job/${encodeURIComponent(job.name)}/set_active`, data)
                .then(() => {
                    job.active = job.active === "true" ? "false" : "true";
                })
                .catch(() => {});
        },
        openCreate() {
            this.createOpen = true;
            this.$nextTick(() => this.$refs.newJobInput?.select());
        },
        create() {
            const data = new FormData();
            data.append("name", this.newJobName);
            axios
                .post("/api/jobs/create", data)
                .then(() => {
                    this.createOpen = false;
                    this.$notify({ text: "New job created", type: "primary" });
                    this.fetch();
                })
                .catch(() => {});
        },
        deleteJob() {
            const name = this.deleting;
            this.deleting = null;
            axios
                .delete(`/api/job/${encodeURIComponent(name)}`)
                .then(() => {
                    this.$notify({ text: `${escapeHtml(name)} has been deleted`, type: "primary" });
                    this.fetch();
                })
                .catch(() => {});
        },
    },
};
</script>

<style scoped>
.pin {
    opacity: 0.45;
}

.pin.on {
    opacity: 1;
    color: var(--primary);
    font-variation-settings: "FILL" 1;
}

.group-title {
    padding: var(--space-2) var(--space-2) var(--space-1);
    font-weight: 600;
}

.job-list {
    border-block-start: 1px solid var(--outline-variant);
}

.job-row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto 11rem auto auto;
    align-items: center;
    gap: var(--space-3);
    min-block-size: 56px;
    padding: var(--space-1) var(--space-2);
    border-block-end: 1px solid var(--outline-variant);
}

.job-row:hover {
    background-color: color-mix(in srgb, var(--on-surface) 4%, transparent);
}

.job-row.inactive .job-main,
.job-row.inactive .job-strip {
    opacity: 0.55;
}

.job-main {
    display: flex;
    flex-direction: column;
    min-inline-size: 0;
}

.job-name {
    color: var(--on-surface);
    font-weight: 500;
}

.job-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    text-align: end;
    white-space: nowrap;
}

.failing {
    color: var(--status-failed);
}

.jobs-empty {
    align-items: center;
    padding: var(--space-8) 0;
    display: flex;
    justify-content: center;
}

.jobs-spinner {
    inline-size: 28px;
    block-size: 28px;
    --_c: var(--primary);
}

@media (max-width: 1024px) {
    .job-row {
        grid-template-columns: auto minmax(0, 1fr) auto auto;
    }
    .job-strip,
    .job-meta {
        display: none;
    }
}
</style>
