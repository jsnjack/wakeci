<template>
    <Card data-cy="feed-item" :data-cy-build="build.id" class="feed-item">
        <ProgressBar class="feed-progress" :type="buildStatus" :progress="(getDoneTasks / getTotalTasks) * 100" />

        <div class="feed-item-content">
            <header class="feed-item-header">
                <router-link class="feed-title" :to="{ name: 'build', params: { id: build.id } }">
                    <span># {{ build.id }}</span>
                    <span>{{ build.name }}</span>
                </router-link>
                <Badge :text="build.status" :type="buildStatus" data-cy="build-status-label" />
            </header>
            <p class="feed-item-info"><span class="material-icons">calendar_today</span> {{ getTimestamp }}</p>
            <p class="feed-item-info">
                <span class="material-icons">timer</span>
                {{ getDoneTasks }}/{{ getTotalTasks }}
                {{ `(${isDone ? "Duration" : "Running for"} ${getDuration})` }}
            </p>

            <template v-if="job">
                <b>Description</b>
                <p>{{ job.desc }}</p>
                <br />
            </template>

            <Badge v-for="param in paramsList" :key="param" :text="param" />

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
		showingParams: {
			type: Array,
			required: false,
		    default: () => [],
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
		paramsList() {
		    return this.build.params?.filter(param => {
				this.showingParams.includes(Object.keys(param)[0]);
			}).map(param => `${Object.keys(param)[0]}=${Object.values(param)[0]}`);
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
.feed-item {
    @apply dark:bg-secondary;
    .feed-progress {
        @apply absolute top-0 right-0 left-0;
    }
    .feed-item-content {
        @apply p-3 pb-0;
        .feed-title {
            @apply flex items-center gap-2 mb-1 dark:text-primary-light;
        }
        .feed-item-header {
            @apply flex items-center gap-6 w-full mb-0;
        }
        .feed-item-info {
            @apply text-gray-border flex items-center text-xs gap-2 m-0 dark:text-white;
            .material-icons {
                @apply text-sm;
            }
        }
        .feed-item-actions {
            @apply flex flex-row gap-1 items-center justify-end;
        }
    }
}

@media (max-width: 480px) {
    .cell-name {
        max-width: 15ch;
}
</style>
