<template>
    <div
        class="feed-group"
        :data-cy-group="root.id"
    >
        <router-link
            v-for="build in shown"
            :key="build.id"
            :to="`/build/${build.id}`"
            class="group-line"
            :style="{ '--depth': build.depth - 1 }"
            :data-deep="build.depth > 2 ? (build.depth > MAX_DEPTH ? 'all' : 'phone') : null"
            :aria-current="build.id === selected ? 'true' : null"
            :data-tip="build.attempts > 1 ? `${build.attempts} attempts` : null"
            data-cy="group-line"
            :data-cy-build="build.id"
            @click="open(build, $event)"
        >
            <StatusIcon
                :status="build.status"
                small
            />
            <span class="muted nums">#{{ build.id }}</span>
            <span class="ellipsis grow"
                >{{ build.name
                }}<span
                    v-if="build.values"
                    class="muted"
                    data-cy="group-values"
                >
                    · {{ build.values }}</span
                ></span
            >
            <span class="via muted">via #{{ build.parent }}</span>
            <span class="muted nums">{{ timeOf(build) }}</span>
        </router-link>
        <button
            v-if="folded"
            type="button"
            class="group-line more"
            :style="{ '--depth': 0 }"
            data-cy="group-more"
            @click="expanded = true"
        >
            {{ foldedText }}
        </button>
        <p
            v-if="root.hiddenMembers"
            class="group-line muted"
            :style="{ '--depth': 0 }"
            data-cy="group-hidden"
        >
            +{{ root.hiddenMembers }} you can't see
        </p>
    </div>
</template>

<script>
import { formatDuration } from "@/analytics.js";
import StatusIcon from "@/components/StatusIcon.vue";
import { groupTree, latestAttempts, MAX_DEPTH, memberValues } from "@/groups.js";

// Lines shown before the ones that passed fold away
const FOLD_AFTER = 10;

// FeedGroup lists, under a root's row in the feed, the builds started under
// it, always, so a failure needs no click (F2 in docs/ui-roadmap.md): a
// tree, each job's latest attempt only. Past 10 lines it lists the builds
// that failed, run or wait, and folds the others into a line that opens
// them.
export default {
    components: { StatusIcon },
    props: {
        // A feed row with members and hiddenMembers
        root: {
            type: Object,
            required: true,
        },
        // Beside the feed, a click shows the build there (F9)
        selectable: {
            type: Boolean,
            default: false,
        },
        selected: {
            type: Number,
            default: null,
        },
        // Each job's { defaults, labels }, for the values a line shows
        jobs: {
            type: Map,
            default: () => new Map(),
        },
    },
    emits: ["select"],
    data() {
        return { expanded: false, MAX_DEPTH };
    },
    computed: {
        lines() {
            return groupTree(this.root.id, latestAttempts(this.root.members || [])).map((build) => ({
                ...build,
                values: memberValues(build, this.root, this.jobs.get(build.name) || null),
            }));
        },
        folds() {
            return !this.expanded && this.lines.length > FOLD_AFTER;
        },
        shown() {
            return this.folds ? this.lines.filter((build) => build.status !== "finished" && build.status !== "skipped") : this.lines;
        },
        folded() {
            return this.folds && this.shown.length < this.lines.length;
        },
        foldedText() {
            const rest = this.lines.filter((build) => !this.shown.includes(build));
            const passed = rest.every((build) => build.status === "finished");
            return `+${rest.length} ${passed ? "passed" : "more"}`;
        },
    },
    methods: {
        open(build, event) {
            if (this.selectable && !event.ctrlKey && !event.metaKey && !event.shiftKey) {
                event.preventDefault();
                this.$emit("select", build.id);
            }
        },
        timeOf(build) {
            if (build.status === "pending") {
                return "queued";
            }
            return build.duration ? formatDuration(build.duration) : build.status;
        },
    },
};
</script>

<style scoped>
.feed-group {
    display: flex;
    flex-direction: column;
    padding-block-end: var(--space-1);
    border-block-end: 1px solid var(--outline-variant);
}

.group-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-block-size: 26px;
    padding-inline: var(--space-2);
    /* Under the root's name, then 22 px a level */
    padding-inline-start: calc(36px + min(var(--depth), 3) * 22px);
    color: var(--on-surface);
    font-size: var(--text-sm);
    border-radius: var(--radius-control);
}

.group-line:hover {
    background-color: color-mix(in srgb, var(--on-surface) 6%, transparent);
    text-decoration: none;
}

.group-line[aria-current="true"] {
    background-color: var(--secondary-container);
}

.group-line.more {
    border: none;
    background: none;
    font: inherit;
    font-size: var(--text-sm);
    color: var(--primary);
    cursor: pointer;
    text-align: start;
}

.via {
    display: none;
}

.group-line[data-deep="all"] .via {
    display: inline;
}

/* Two levels on phones */
@media (max-width: 600px) {
    .group-line {
        padding-inline-start: calc(36px + min(var(--depth), 1) * 22px);
    }

    .group-line[data-deep] .via {
        display: inline;
    }
}
</style>
