<template>
    <div
        v-if="history.length > 1"
        class="job-history cluster nowrap gap-2"
        data-cy="job-history"
    >
        <OutcomeStrip
            :builds="history"
            :slots="history.length"
            :current="buildId"
            :to="buildPath"
        />
        <!-- Also , and . -->
        <span class="cluster nowrap gap-0">
            <button
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                :disabled="!neighbours.older"
                :aria-label="`Older build of ${jobName}`"
                :data-tip="neighbours.older ? `Older build of ${jobName} (,)` : null"
                data-cy="job-older"
                @click="open(neighbours.older)"
            >
                <i class="icon small">chevron_left</i>
            </button>
            <button
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                :disabled="!neighbours.newer"
                :aria-label="`Newer build of ${jobName}`"
                :data-tip="neighbours.newer ? `Newer build of ${jobName} (.)` : null"
                data-cy="job-newer"
                @click="open(neighbours.newer)"
            >
                <i class="icon small">chevron_right</i>
            </button>
        </span>
    </div>
</template>

<script>
import OutcomeStrip from "@/components/charts/OutcomeStrip.vue";
import axios from "axios";

const SHOWN = 20;

function outcome(build) {
    return { id: build.id, status: build.status, startedAt: build.startedAt, duration: build.duration };
}

// JobHistory shows the job's last builds next to a build's title, and moves
// to the build before or after it with , and . (B7 in docs/ui-roadmap.md)
export default {
    components: { OutcomeStrip },
    props: {
        jobName: {
            type: String,
            required: true,
        },
        buildId: {
            type: Number,
            required: true,
        },
        // The build's status, so its bar follows it
        status: {
            type: String,
            default: "",
        },
        // Beside the feed (F9), whose links keep the feed
        embedded: {
            type: Boolean,
            default: false,
        },
    },
    data() {
        // history is oldest first; older and newer are the builds of the job
        // around this one, when it is older than the strip
        return { history: [], older: null, newer: null };
    },
    computed: {
        neighbours() {
            const i = this.history.findIndex((build) => build.id === this.buildId);
            if (i < 0) {
                return { older: this.older, newer: this.newer };
            }
            return { older: this.history[i - 1]?.id ?? this.older, newer: this.history[i + 1]?.id ?? null };
        },
    },
    watch: {
        status(status) {
            this.history = this.history.map((build) => (build.id === this.buildId ? { ...build, status } : build));
        },
    },
    mounted() {
        // A key pressed while the builds load waits for them
        this.loaded = this.fetch();
        document.addEventListener("keydown", this.onKey);
    },
    unmounted() {
        this.gone = true;
        document.removeEventListener("keydown", this.onKey);
    },
    methods: {
        feed(query) {
            return axios.get(`/api/feed?job=${encodeURIComponent(this.jobName)}&${query}`).then((response) => response.data || []);
        },
        fetch() {
            return this.feed(`limit=${SHOWN}`)
                .then((builds) => {
                    this.history = builds.map(outcome).reverse();
                    if (this.history.some((build) => build.id === this.buildId)) {
                        // The one before the oldest shown
                        if (this.history[0]?.id === this.buildId) {
                            return this.feed(`before=${this.buildId}&limit=1`).then((older) => (this.older = older[0]?.id ?? null));
                        }
                        return null;
                    }
                    return Promise.all([
                        this.feed(`before=${this.buildId}&limit=1`).then((older) => (this.older = older[0]?.id ?? null)),
                        this.feed(`after=${this.buildId}&limit=200`).then((newer) => (this.newer = newer.length ? Math.min(...newer.map((b) => b.id)) : null)),
                    ]);
                })
                .catch(() => {});
        },
        onKey(event) {
            if (event.ctrlKey || event.altKey || event.metaKey || ![",", "."].includes(event.key)) {
                return;
            }
            if (event.target.closest?.("input, textarea, select, [contenteditable='true']")) {
                return;
            }
            // The key is the page's, so Firefox's "search when you start
            // typing" doesn't take it
            event.preventDefault();
            const key = event.key;
            this.loaded.then(() => {
                const id = key === "," ? this.neighbours.older : this.neighbours.newer;
                // Not once the page closed, e.g. on a second press
                if (id && !this.gone) {
                    this.$router.push(this.buildPath(id));
                }
            });
        },
        open(id) {
            if (id) {
                this.$router.push(this.buildPath(id));
            }
        },
        buildPath(id) {
            if (this.embedded) {
                return { path: "/", query: { ...this.$route.query, build: id } };
            }
            return `/build/${id}`;
        },
    },
};
</script>

<style scoped>
@media (max-width: 600px) {
    .job-history {
        display: none;
    }
}
</style>
