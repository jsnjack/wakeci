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
        <span
            v-if="siblings.length > 1"
            class="muted keys"
            ><kbd>[</kbd> <kbd>]</kbd> the builds beside it</span
        >
    </nav>
    <section
        v-else-if="isRoot"
        class="panel stack gap-1"
        aria-labelledby="group-title"
        data-cy="build-group"
    >
        <h2
            id="group-title"
            class="card-title"
        >
            Started {{ startedCount }} {{ startedCount === 1 ? "build" : "builds" }}
        </h2>
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
    </section>
</template>

<script>
import { formatDuration } from "@/analytics.js";
import StatusIcon from "@/components/StatusIcon.vue";
import { groupTree, latestAttempts, MAX_DEPTH, memberValues } from "@/groups.js";
import axios from "axios";
import vuex from "vuex";

// Every build's updates, which the group's come with
const TOPIC = "build:update:";

// BuildGroup shows the builds started under the same first build (F2 in
// docs/ui-roadmap.md): on a member's page "Part of #5115 push" with the
// group's strip, where [ and ] move between the builds its parent started;
// on the root's page the builds it started, as a tree
export default {
    components: { StatusIcon },
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
        return { group: { root: 0, builds: [], hidden: 0 }, jobs: new Map(), subscribed: false, MAX_DEPTH };
    },
    computed: {
        ...vuex.mapState(["ws"]),
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
            const i = this.siblings.findIndex((build) => build.id === this.buildId);
            const next = this.siblings[i + (event.key === "]" ? 1 : -1)];
            if (i >= 0 && next) {
                this.$router.push(this.buildPath(next.id));
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

.keys kbd {
    padding: 0 4px;
    border: 1px solid var(--outline-variant);
    border-radius: 4px;
    font-family: var(--font-mono);
    font-size: 0.85em;
}

/* No keyboard to press them on */
@media (pointer: coarse), (max-width: 600px) {
    .keys {
        display: none;
    }
    .strip-build {
        padding: 4px;
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
