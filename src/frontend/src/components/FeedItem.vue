<template>
    <Card data-cy="feed-item" :data-cy-build="build.id">
        <ProgressBar class="feed-progress" :type="buildStatus" :progress="(getDoneTasks / getTotalTasks) * 100" />

        <div class="feed-item-content">
            <header class="feed-item-header">
                <router-link class="feed-title" :to="{ name: 'build', params: { id: build.id } }">
                    <span># {{ build.id }}</span>
                    <span>{{ build.name }}</span>
                </router-link>
                <Badge :text="build.status" :type="buildStatus" data-cy="build-status-label" />
            </header>

            <template v-if="job">
                <b>Description</b>
                <p>{{ job.desc }}</p>
                <br />
            </template>

            <div class="feed-item-info">
                <p><b>Params:</b> {{ getParamsString }}</p>
                <p>
                    <b>Tasks:</b> {{ getDoneTasks }}/{{ getTotalTasks }}
                    {{ `(${isDone ? "Duration" : "Running for"} ${getDuration})` }}
                </p>
                <p><b>Timestamp:</b> {{ getTimestamp }}</p>
            </div>

            <div class="feed-item-actions">
                <button
                    :to="{ name: 'build', params: { id: build.id } }"
                    v-if="!isDone"
                    class="btn btn-danger small float-right ml-3"
                    data-cy="abort-build-button"
                    @click="abort"
                >
                    Abort
                </button>

                <router-link
                    :to="{ name: 'build', params: { id: build.id } }"
                    class="btn btn-secondary small float-right"
                    data-cy="open-build-button"
                    v-if="showOpen"
                >
                    Open
                </router-link>
            </div>
        </div>
    </Card>
</template>

<script>
import axios from "axios";
import dayjs from "dayjs";
import Card from "./ui/Card.vue";
import Badge from "./ui/Badge.vue";
import ProgressBar from "./ui/ProgressBar.vue";
import { paramsToString } from "@/utils/params";
import { runningDuration, doneDuration, startedAtRelative } from "@/utils/duration";

export default {
    components: { Card, Badge, ProgressBar },
    props: {
        build: {
            type: Object,
            required: true,
        },
        showOpen: {
            type: Boolean,
            default: true,
        },
        job: {
            type: Object,
            required: false,
        },
    },
    computed: {
        getMainTasks() {
            return this.build.tasks.filter((item) => {
                return item.kind === "main";
            });
        },
        getDoneTasks() {
            return this.getMainTasks.filter((item) => {
                return item.status !== "pending" && item.status !== "running";
            }).length;
        },
        getTotalTasks() {
            return this.getMainTasks.length;
        },
        getAbortURL() {
            return `/api/build/${this.build.id}/abort`;
        },
        getStartURL() {
            return `/api/build/${this.build.id}/start`;
        },
        buildStatus() {
            const status = {
                running: "info",
                "timed out": "info",
                finished: "success",
                failed: "danger",
                aborted: "warning",
            };
            return status[this.build.status];
        },
        isDone() {
            switch (this.build.status) {
                case "failed":
                case "finished":
                case "aborted":
                case "skipped":
                case "timed out":
                    return true;
            }
            return false;
        },
        getParamsString() {
            if (this.build.params) {
                return paramsToString(this.build.params);
            }
            return "-";
        },
        getDuration() {
            return this.isDone ? doneDuration(this.build.duration) : runningDuration(this.build.startedAt);
        },
        getTimestamp() {
            const startedAt = dayjs(this.build.startedAt);
            return `${startedAt.format("DD-MM-YYYY HH:mm:ss")} - (${startedAtRelative(startedAt)})`;
        },
    },
    methods: {
        abort(event) {
            if (window.confirm("Are you sure you want to abort this build?")) {
                axios
                    .post(this.getAbortURL)
                    .then((response) => {
                        this.$notify({
                            text: `${this.build.id} has been aborted`,
                            type: "warn",
                        });
                    })
                    .catch((error) => {});
            }
        },
        start(event) {
            axios
                .post(event.target.href || event.target.parentElement.href)
                .then((response) => {
                    this.$notify({ text: `${this.build.id} has been started`, type: "success" });
                })
                .catch((error) => {});
        },
    },
};
</script>

<style scoped lang="scss">
.feed-progress {
    @apply absolute top-0 right-0 left-0;
}
.feed-item-content {
    @apply p-3;
    .feed-title {
        @apply flex items-center gap-2;
    }
    .feed-item-header {
        @apply flex items-center gap-6 w-full mb-4;
    }
    .feed-item-info {
        @apply text-sm flex md:items-center md:gap-4 w-full md:flex-row flex-col gap-2 items-start;
    }
    .feed-item-actions {
        @apply flex flex-row gap-1 items-center justify-end;
    }
}

@media (max-width: 480px) {
    .cell-name {
        max-width: 15ch;
}
</style>
