<template>
    <div
        class="feed-row"
        :class="{ compact, 'has-group': members.length || build.hiddenMembers, flash }"
        :data-cy-build="build.id"
        :aria-current="selected ? 'true' : null"
        @click="openFromRow"
    >
        <StatusIcon
            :status="shownStatus"
            tip
            class="tip-start"
        />
        <div class="feed-main">
            <router-link
                :to="buildPath"
                class="feed-name ellipsis"
                data-cy="open-build-button"
                @click="onNameClick"
            >
                <span class="muted nums">#{{ build.id }}</span> {{ build.name }}
            </router-link>
            <span
                v-if="shownStatus === 'running' && failedMembers"
                class="group-failed text-sm"
                data-cy="group-failed"
                >{{ failedMembers }} failed</span
            >
            <div
                v-if="values.length"
                class="feed-values muted text-sm"
            >
                <template
                    v-for="(item, i) in shownValues"
                    :key="item.name"
                >
                    <span
                        v-if="i"
                        aria-hidden="true"
                        >·</span
                    >
                    <button
                        type="button"
                        class="feed-value"
                        data-cy="params-value"
                        :data-tip="`${item.name}: ${item.value}`"
                        :aria-label="`Search for ${item.name} ${item.value}`"
                        @click.stop="search(item, $event)"
                    >
                        {{ item.short }}
                    </button>
                </template>
                <span v-if="values.length > shownValues.length">· +{{ values.length - shownValues.length }}</span>
            </div>
        </div>
        <div
            v-if="!compact"
            class="feed-meta muted text-sm nums"
        >
            <template v-if="build.status === 'pending'">
                <span data-cy="queue-place">{{ queuePlace }}</span>
                <span data-cy="started-ago">{{ waited }}</span>
            </template>
            <template v-else>
                <span data-cy="duration">{{ durationText }}</span>
                <span
                    data-cy="started-ago"
                    :data-tip="startedAtText"
                    class="tip-end"
                    >{{ startedAgo }}</span
                >
            </template>
        </div>
        <div
            v-if="!compact"
            class="feed-actions"
        >
            <button
                v-if="canAbort"
                type="button"
                class="btn btn-ghost btn-icon"
                data-cy="abort-build-button"
                :aria-label="members.length ? 'Abort all' : 'Abort'"
                :data-tip="members.length ? 'Abort it and the builds it started' : 'Abort'"
                @click.stop="abort"
            >
                <i class="icon">stop</i>
            </button>
            <button
                v-if="canStartNow"
                type="button"
                class="btn btn-ghost btn-icon"
                data-cy="start-build-button"
                aria-label="Start now"
                data-tip="Start now, past the limit of concurrent builds"
                @click.stop="startNow"
            >
                <i class="icon">play_arrow</i>
            </button>
            <button
                v-if="canRunAgain"
                type="button"
                class="btn btn-ghost btn-icon"
                data-cy="run-again-button"
                aria-label="Run again"
                data-tip="Run again with these parameters"
                @click.stop="runAgain"
            >
                <i class="icon">replay</i>
            </button>
            <a
                :href="buildPath"
                target="_blank"
                rel="noopener"
                class="btn btn-ghost btn-icon tip-end"
                aria-label="Open in a new tab"
                data-tip="Open in a new tab"
                @click.stop
            >
                <i class="icon">open_in_new</i>
            </a>
        </div>
        <UiMenu
            v-if="!compact"
            class="feed-menu"
            label="Build actions"
        >
            <button
                v-if="canAbort"
                type="button"
                role="menuitem"
                @click.stop="abort"
            >
                <i class="icon">stop</i> {{ members.length ? "Abort all" : "Abort" }}
            </button>
            <button
                v-if="canStartNow"
                type="button"
                role="menuitem"
                @click.stop="startNow"
            >
                <i class="icon">play_arrow</i> Start now
            </button>
            <button
                v-if="canRunAgain"
                type="button"
                role="menuitem"
                @click.stop="runAgain"
            >
                <i class="icon">replay</i> Run again
            </button>
            <a
                :href="buildPath"
                target="_blank"
                rel="noopener"
                role="menuitem"
                @click.stop
            >
                <i class="icon">open_in_new</i> Open in a new tab
            </a>
        </UiMenu>
        <div
            v-if="build.status === 'running'"
            class="feed-progress-track"
            aria-hidden="true"
        >
            <div
                class="feed-progress"
                :class="{ unknown: progress === null }"
                :style="progress === null ? null : { inlineSize: `${progress}%` }"
            ></div>
        </div>
    </div>
</template>

<script>
import { ago, dateTime, formatDuration, isZeroTime } from "@/analytics.js";
import StatusIcon from "@/components/StatusIcon.vue";
import UiMenu from "@/components/ui/UiMenu.vue";
import { MAX_ROW_VALUES, ordinal, rowValues } from "@/feed.js";
import { groupStatus, isFailure, latestAttempts } from "@/groups.js";
import { escapeHtml, searchTerm } from "@/store/utils.js";
import axios from "axios";
import vuex from "vuex";

const NS = 1e9;

// A build in the feed: two plain lines, the status, ID and job on the first,
// the labels and changed params on the second; duration and time on the
// right. The row opens the build; its actions show on hover and focus, and
// in a menu on phones, only when they can act.
export default {
    components: { StatusIcon, UiMenu },
    props: {
        build: {
            type: Object,
            required: true,
        },
        // The job's { defaults, labels }, null while unknown
        job: {
            type: Object,
            default: null,
        },
        // The time, which the feed moves on every second
        now: {
            type: Number,
            required: true,
        },
        // A click selects the build, to show it beside the feed (F9),
        // instead of opening its page
        selectable: {
            type: Boolean,
            default: false,
        },
        selected: {
            type: Boolean,
            default: false,
        },
        // Two short lines, for the list beside a build
        compact: {
            type: Boolean,
            default: false,
        },
    },
    emits: ["setFilter", "select"],
    data() {
        // A short highlight when the status changes, since the row stays
        // where it is (L1)
        return { flash: false };
    },
    computed: {
        ...vuex.mapGetters(["can"]),
        ...vuex.mapState(["queue"]),
        buildPath() {
            return `/build/${this.build.id}`;
        },
        values() {
            return rowValues(this.build.params, this.job);
        },
        shownValues() {
            return this.values.slice(0, MAX_ROW_VALUES);
        },
        isDone() {
            return ["failed", "finished", "aborted", "skipped", "timed out"].includes(this.build.status);
        },
        // The builds started under this one, each job's latest attempt (F2)
        members() {
            return latestAttempts(this.build.members || []);
        },
        // A root is running while its group runs, and then shows the worst
        // result of all
        shownStatus() {
            return this.members.length ? groupStatus([this.build, ...this.members]) : this.build.status;
        },
        failedMembers() {
            return this.members.filter(isFailure).length;
        },
        canRun() {
            return this.can("run", this.build.name);
        },
        canAbort() {
            return this.canRun && (!this.isDone || this.shownStatus === "running");
        },
        canStartNow() {
            return this.canRun && this.build.status === "pending" && this.queue.queuedIDs.includes(this.build.id);
        },
        canRunAgain() {
            return this.canRun && this.isDone;
        },
        started() {
            return !isZeroTime(this.build.startedAt);
        },
        // How long it took, or runs so far
        durationText() {
            if (this.build.status === "running" && this.started) {
                return formatDuration(this.elapsed * NS);
            }
            return this.build.duration ? formatDuration(this.build.duration) : "";
        },
        elapsed() {
            return Math.max(0, (this.now - new Date(this.build.startedAt).getTime()) / 1000);
        },
        // Running time against the usual duration, null without one
        progress() {
            if (!this.build.eta || !this.started) {
                return null;
            }
            return Math.min(100, (this.elapsed / (this.build.eta / NS)) * 100);
        },
        startedAgo() {
            // `now` makes it move on
            return this.now && this.started ? ago(this.build.startedAt) : "";
        },
        startedAtText() {
            return this.started ? dateTime(this.build.startedAt) : null;
        },
        queuePlace() {
            const i = this.queue.queuedIDs.indexOf(this.build.id);
            return i < 0 ? "queued" : `${ordinal(i + 1)} in queue`;
        },
        waited() {
            if (isZeroTime(this.build.queuedAt)) {
                return "";
            }
            const seconds = Math.max(0, (this.now - new Date(this.build.queuedAt).getTime()) / 1000);
            return `waiting ${formatDuration(seconds * NS)}`;
        },
    },
    watch: {
        shownStatus(status, old) {
            if (old && status !== old) {
                this.flash = false;
                this.$nextTick(() => {
                    this.flash = true;
                });
                clearTimeout(this.flashTimer);
                this.flashTimer = setTimeout(() => (this.flash = false), 1500);
            }
        },
    },
    unmounted() {
        clearTimeout(this.flashTimer);
    },
    methods: {
        // A click anywhere on the row opens the build, unless it was on a
        // control or selected text
        openFromRow(event) {
            if (event.target.closest("a, button, .menu") || window.getSelection()?.toString()) {
                return;
            }
            if (event.ctrlKey || event.metaKey) {
                window.open(this.buildPath, "_blank", "noopener");
                return;
            }
            if (this.selectable) {
                this.$emit("select", this.build.id);
                return;
            }
            this.$router.push(this.buildPath);
        },
        // The name is a link, so it opens in a new tab as links do; a plain
        // click selects the build where the feed shows builds beside it
        onNameClick(event) {
            if (this.selectable && !event.ctrlKey && !event.metaKey && !event.shiftKey && event.button === 0) {
                event.preventDefault();
                this.$emit("select", this.build.id);
            }
        },
        search(item, event) {
            this.$emit("setFilter", { term: searchTerm(item.name, item.value), append: event.ctrlKey || event.metaKey });
        },
        abort() {
            const group = this.members.length > 0;
            axios
                .post(`/api/build/${this.build.id}/abort${group ? "?group=1" : ""}`)
                .then(() =>
                    this.$notify({
                        text: group ? `#${this.build.id} and the builds it started have been aborted` : `#${this.build.id} has been aborted`,
                        type: "primary",
                    }),
                )
                .catch(() => {});
        },
        startNow() {
            axios
                .post(`/api/build/${this.build.id}/start`)
                .then(() => this.$notify({ text: `#${this.build.id} has been started`, type: "primary" }))
                .catch(() => {});
        },
        runAgain() {
            const params = new URLSearchParams((this.build.params || []).map((param) => Object.entries(param)[0]));
            // A build that another build started stays under its parent
            if (this.build.parent) {
                params.set("WAKE_AGAIN", this.build.id);
            }
            axios
                .post(`/api/job/${encodeURIComponent(this.build.name)}/run?${params}`)
                .then((response) => {
                    const id = escapeHtml(response.data);
                    this.$notify({
                        text: `${escapeHtml(this.build.name)} has been scheduled (<a href="/build/${id}/">#${id}</a>)`,
                        type: "primary",
                        duration: 10000,
                    });
                })
                .catch(() => {});
        },
    },
};
</script>

<style scoped>
.feed-row {
    position: relative;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto auto;
    align-items: center;
    gap: var(--space-3);
    min-block-size: 60px;
    padding: var(--space-2) var(--space-3);
    border-block-end: 1px solid var(--outline-variant);
    cursor: pointer;
}

.feed-row.flash {
    animation: row-flash 1.5s ease-out;
}

@keyframes row-flash {
    from {
        background-color: color-mix(in srgb, var(--primary) 18%, transparent);
    }
}

@media (prefers-reduced-motion: reduce) {
    .feed-row.flash {
        animation: none;
        outline: 2px solid color-mix(in srgb, var(--primary) 40%, transparent);
        outline-offset: -2px;
    }
}

.feed-row.has-group {
    border-block-end: none;
}

.group-failed {
    color: var(--status-failed);
    font-weight: 600;
}

.feed-row:hover {
    background-color: color-mix(in srgb, var(--on-surface) 4%, transparent);
}

.feed-row[aria-current="true"] {
    background-color: var(--secondary-container);
}

.feed-row.compact {
    grid-template-columns: auto minmax(0, 1fr);
    min-block-size: 48px;
}

.feed-main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-inline-size: 0;
}

.feed-name {
    color: var(--on-surface);
    font-weight: 500;
}

.feed-values {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-1);
    align-items: baseline;
}

.feed-value {
    padding: 0;
    border: none;
    background: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
}

.feed-value:hover {
    color: var(--on-surface);
    text-decoration: underline;
}

.feed-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    min-inline-size: 7rem;
    text-align: end;
    white-space: nowrap;
}

.feed-actions {
    display: flex;
    gap: 2px;
    opacity: 0;
}

.feed-row:hover .feed-actions,
.feed-row:focus-within .feed-actions {
    opacity: 1;
}

.feed-menu {
    display: none;
}

/* Along the bottom edge: the running time against the usual duration, or a
   moving stripe without one */
.feed-progress-track {
    position: absolute;
    inset-inline: 0;
    inset-block-end: -1px;
    block-size: 2px;
    overflow: hidden;
}

.feed-progress {
    position: absolute;
    inset-inline-start: 0;
    block-size: 100%;
    background-color: var(--status-running);
    transition: inline-size 1s linear;
}

.feed-progress.unknown {
    inline-size: 30%;
    animation: feed-progress 1.6s ease-in-out infinite;
}

@keyframes feed-progress {
    from {
        inset-inline-start: -30%;
    }
    to {
        inset-inline-start: 100%;
    }
}

@media (prefers-reduced-motion: reduce) {
    .feed-progress.unknown {
        animation: none;
        inline-size: 100%;
        opacity: 0.5;
    }
}

@media (max-width: 600px) {
    .feed-row {
        grid-template-columns: auto minmax(0, 1fr) auto auto;
        gap: var(--space-2);
        padding-inline: var(--space-2);
    }
    .feed-meta {
        min-inline-size: 0;
    }
    .feed-actions {
        display: none;
    }
    .feed-menu {
        display: inline-flex;
    }
}
</style>
