<template>
    <nav
        v-if="isMember"
        class="group-strip cluster gap-2 text-sm"
        aria-label="Builds of the group"
        data-cy="build-group"
    >
        <span class="muted">Part of</span>
        <router-link
            :to="buildPath(group.root)"
            data-cy="group-root"
            >{{ root ? `#${group.root} ${root.name}` : `#${group.root}` }}</router-link
        >
        <span class="cluster gap-1">
            <router-link
                v-for="build in group.builds"
                :key="build.id"
                :to="buildPath(build.id)"
                class="strip-build"
                :aria-current="build.id === buildId ? 'page' : null"
                :aria-label="label(build)"
                :data-tip="label(build)"
                data-cy="group-build"
            >
                <StatusIcon
                    :status="build.status"
                    small
                />
            </router-link>
        </span>
        <span
            v-if="group.hidden"
            class="muted"
            data-cy="group-hidden"
            >+{{ group.hidden }} you can't see</span
        >
        <!-- The builds its parent started, one by one; also [ and ] -->
        <span
            v-if="siblings.length > 1"
            class="cluster nowrap gap-0"
        >
            <button
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                :disabled="!previousSibling"
                :aria-label="`Previous build started by #${parent}`"
                :data-tip="previousSibling ? `Previous build started by #${parent} ([)` : null"
                data-cy="sibling-previous"
                @click="go(previousSibling)"
            >
                <i class="icon small">chevron_left</i>
            </button>
            <button
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                :disabled="!nextSibling"
                :aria-label="`Next build started by #${parent}`"
                :data-tip="nextSibling ? `Next build started by #${parent} (])` : null"
                data-cy="sibling-next"
                @click="go(nextSibling)"
            >
                <i class="icon small">chevron_right</i>
            </button>
        </span>
    </nav>
    <section
        v-else-if="isRoot"
        class="panel stack gap-1"
        aria-labelledby="group-title"
        data-cy="build-group"
    >
        <div class="cluster between gap-2">
            <h2
                id="group-title"
                class="card-title"
            >
                Started {{ startedCount }} {{ startedCount === 1 ? "build" : "builds" }}
            </h2>
            <button
                v-if="failedToRun.length"
                type="button"
                class="btn btn-sm"
                data-cy="group-run-failed"
                @click="confirmOpen = true"
            >
                <i class="icon small">replay</i> Run again the {{ failedToRun.length }} failed
            </button>
        </div>
        <router-link
            v-for="build in members"
            :key="build.id"
            :to="buildPath(build.id)"
            class="group-row cluster nowrap gap-2"
            :style="{ '--depth': build.depth - 1 }"
            :data-deep="build.depth > 2 ? (build.depth > MAX_DEPTH ? 'all' : 'phone') : null"
            data-cy="group-build"
        >
            <StatusIcon
                :status="build.status"
                small
            />
            <span class="nums">#{{ build.id }}</span>
            <span class="grow ellipsis"
                >{{ build.name
                }}<span
                    v-if="build.values"
                    class="muted"
                    data-cy="group-values"
                >
                    · {{ build.values }}</span
                ></span
            >
            <span
                v-if="build.attempts > 1"
                class="muted text-sm"
                data-cy="group-attempts"
                >{{ build.attempts }} attempts</span
            >
            <span class="via muted text-sm">via #{{ build.parent }}</span>
            <span class="muted text-sm nums">{{ timeOf(build) }}</span>
        </router-link>
        <p
            v-if="group.hidden"
            class="muted text-sm"
            data-cy="group-hidden"
        >
            +{{ group.hidden }} you can't see
        </p>
        <UiDialog
            v-model:open="confirmOpen"
            :title="`Run again the ${failedToRun.length} failed`"
        >
            <p>Each runs again with its own values, under the build that started it.</p>
            <ul
                class="stack gap-1"
                data-cy="group-run-failed-list"
            >
                <li
                    v-for="build in failedToRun"
                    :key="build.id"
                >
                    #{{ build.id }} {{ build.name
                    }}<span
                        v-if="build.values"
                        class="muted"
                    >
                        · {{ build.values }}</span
                    >
                </li>
            </ul>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="confirmOpen = false"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    class="btn btn-primary"
                    data-cy="group-run-failed-confirm"
                    @click="runFailed"
                >
                    Run again
                </button>
            </template>
        </UiDialog>
    </section>
</template>

<script>
import { formatDuration } from "@/analytics.js";
import StatusIcon from "@/components/StatusIcon.vue";
import UiDialog from "@/components/ui/UiDialog.vue";
import { runValues } from "@/feed.js";
import { escapeHtml } from "@/store/utils.js";
import { groupTree, isFailure, latestAttempts, MAX_DEPTH, memberValues } from "@/groups.js";
import axios from "axios";
import vuex from "vuex";

// Every build's updates, which the group's come with
const TOPIC = "build:update:";

// BuildGroup shows the builds started under the same first build (F2 in
// docs/ui-roadmap.md): on a member's page "Part of #5115 push" with the
// group's strip, where [ and ] move between the builds its parent started;
// on the root's page the builds it started, as a tree
export default {
    components: { StatusIcon, UiDialog },
    props: {
        buildId: {
            type: Number,
            required: true,
        },
        // The build that started this one, 0 if none
        parent: {
            type: Number,
            default: 0,
        },
        done: {
            type: Boolean,
            default: false,
        },
        // Beside the feed (F9), which already gets every build's updates,
        // and whose links keep the feed
        embedded: {
            type: Boolean,
            default: false,
        },
    },
    data() {
        // jobs: each job's { defaults, labels }, so lines leave out values at
        // their default, as the feed does
        return { group: { root: 0, builds: [], hidden: 0 }, jobs: new Map(), subscribed: false, confirmOpen: false, MAX_DEPTH };
    },
    computed: {
        ...vuex.mapState(["ws"]),
        ...vuex.mapGetters(["can"]),
        isRoot() {
            return this.group.root !== 0 && this.group.root === this.buildId;
        },
        isMember() {
            return this.group.root !== 0 && this.group.root !== this.buildId;
        },
        root() {
            return this.group.builds.find((build) => build.id === this.group.root);
        },
        // Every build under the root, attempts too
        startedCount() {
            return this.group.builds.filter((build) => build.id !== this.group.root).length;
        },
        // Each job's latest attempt, as in the feed
        members() {
            const builds = latestAttempts(this.group.builds.filter((build) => build.id !== this.group.root));
            return groupTree(this.group.root, builds).map((build) => ({ ...build, values: memberValues(build, this.root, this.jobs.get(build.name) || null) }));
        },
        // The latest attempts that failed or timed out, of the jobs the user
        // may run; aborted ones were stopped on purpose
        failedToRun() {
            return this.members.filter((build) => isFailure(build) && this.can("run", build.name));
        },
        previousSibling() {
            return this.siblings[this.siblings.findIndex((build) => build.id === this.buildId) - 1] || null;
        },
        nextSibling() {
            const i = this.siblings.findIndex((build) => build.id === this.buildId);
            return i < 0 ? null : this.siblings[i + 1] || null;
        },
        siblings() {
            return this.group.builds.filter((build) => build.id !== this.group.root && build.parent === this.parent);
        },
        // The group can still change while the build runs, since it may
        // start builds, and once it has a group, whose members can run again
        follows() {
            return this.group.root !== 0 || !this.done;
        },
    },
    watch: {
        follows: "updateSubscription",
        "ws.reconnects": "fetch",
    },
    mounted() {
        this.fetch();
        this.emitter.on(TOPIC, this.apply);
        document.addEventListener("keydown", this.onKey);
        this.updateSubscription();
    },
    unmounted() {
        this.emitter.off(TOPIC, this.apply);
        document.removeEventListener("keydown", this.onKey);
        if (this.subscribed) {
            this.$store.commit("UNSUBSCRIBE_TOPIC", TOPIC);
        }
    },
    methods: {
        // runFailed runs each failed build again with its values; WAKE_AGAIN
        // keeps it under its parent, also once the parent ended
        async runFailed() {
            this.confirmOpen = false;
            const started = [];
            const refused = [];
            for (const build of this.failedToRun) {
                const query = new URLSearchParams(runValues(build.params, this.jobs.get(build.name)?.defaults || null));
                query.set("WAKE_AGAIN", build.id);
                try {
                    const response = await axios.post(`/api/job/${encodeURIComponent(build.name)}/run?${query}`);
                    started.push(response.data);
                } catch {
                    refused.push(build.id);
                }
            }
            const links = started.map((id) => `<a href="/build/${escapeHtml(id)}/">#${escapeHtml(id)}</a>`).join(", ");
            this.$notify({
                text: refused.length
                    ? `Started ${links || "none"}; #${refused.join(", #")} couldn't run again`
                    : `Started ${started.length} ${started.length === 1 ? "build" : "builds"} again: ${links}`,
                type: refused.length ? "error" : "primary",
                duration: 10000,
            });
            // Shows the new attempts at once, live updates or not
            this.fetch();
        },
        fetch() {
            axios
                .get(`/api/build/${this.buildId}/group`)
                .then((response) => {
                    this.group = response.data;
                    if (this.group.root && this.jobs.size === 0) {
                        this.fetchJobs();
                    }
                })
                .catch(() => {});
        },
        fetchJobs() {
            axios
                .get("/api/jobs/")
                .then((response) => {
                    this.jobs = new Map(
                        (response.data || []).map((job) => [job.name, { defaults: Object.assign({}, ...(job.defaultParams || [])), labels: job.labels || [] }]),
                    );
                })
                .catch(() => {});
        },
        // Counted, so it doesn't end the feed's subscription beside it (F9)
        updateSubscription() {
            if (this.follows === this.subscribed) {
                return;
            }
            this.$store.commit(this.follows ? "SUBSCRIBE_TOPIC" : "UNSUBSCRIBE_TOPIC", TOPIC);
            this.subscribed = this.follows;
        },
        apply(ev) {
            const root = this.group.root || this.buildId;
            if (ev.id !== root && ev.root !== root) {
                return;
            }
            // The first build it started: the group has a root now
            if (!this.group.root) {
                this.fetch();
                return;
            }
            const build = {
                id: ev.id,
                name: ev.name,
                status: ev.status,
                parent: ev.parent || 0,
                queuedAt: ev.queuedAt,
                startedAt: ev.startedAt,
                duration: ev.duration,
                params: ev.params || [],
            };
            const builds = this.group.builds.filter((item) => item.id !== ev.id);
            builds.push(build);
            builds.sort((a, b) => a.id - b.id);
            this.group = { ...this.group, builds };
        },
        onKey(event) {
            if (!this.isMember || event.ctrlKey || event.altKey || event.metaKey || !["[", "]"].includes(event.key)) {
                return;
            }
            if (event.target.closest?.("input, textarea, select, [contenteditable='true']")) {
                return;
            }
            if (this.siblings.length < 2) {
                return;
            }
            // The key is the page's, so Firefox's "search when you start
            // typing" doesn't take it
            event.preventDefault();
            this.go(event.key === "]" ? this.nextSibling : this.previousSibling);
        },
        go(build) {
            if (build) {
                this.$router.push(this.buildPath(build.id));
            }
        },
        buildPath(id) {
            if (this.embedded) {
                return { path: "/", query: { ...this.$route.query, build: id } };
            }
            return `/build/${id}`;
        },
        label(build) {
            const values = build.id === this.group.root ? "" : memberValues(build, this.root, this.jobs.get(build.name) || null);
            return `#${build.id} ${build.name}${values ? ` · ${values}` : ""}: ${build.status}`;
        },
        timeOf(build) {
            return build.duration ? formatDuration(build.duration) : build.status;
        },
    },
};
</script>

<style scoped>
.strip-build {
    display: inline-flex;
    padding: 2px;
    border-radius: var(--radius-control);
}

.strip-build:hover {
    background-color: color-mix(in srgb, var(--on-surface) 8%, transparent);
}

.strip-build[aria-current="page"] {
    outline: 2px solid var(--primary);
    outline-offset: -1px;
}

/* No keyboard to press them on */
@media (pointer: coarse), (max-width: 600px) {
    .strip-build {
        padding: 4px;
    }
    .group-strip > a {
        display: inline-block;
        padding-block: 2px;
    }
}

.group-row {
    padding: var(--space-1) var(--space-2);
    padding-inline-start: calc(var(--space-2) + min(var(--depth), 3) * 22px);
    border-radius: var(--radius-control);
    color: var(--on-surface);
}

.group-row:hover {
    background-color: color-mix(in srgb, var(--on-surface) 8%, transparent);
    text-decoration: none;
}

.via {
    display: none;
}

.group-row[data-deep="all"] .via {
    display: inline;
}

/* Two levels on phones */
@media (max-width: 600px) {
    .group-row {
        padding-inline-start: calc(var(--space-2) + min(var(--depth), 1) * 22px);
    }

    .group-row[data-deep] .via {
        display: inline;
    }
}
</style>
