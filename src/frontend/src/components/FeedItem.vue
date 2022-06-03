<template>
    <Card>
        <ProgressBar class="feed-progress" :type="buildStatus" :progress="(getDoneTasks / getTotalTasks) * 100" />

        <div class="feed-item-content">
            <router-link class="feed-head" :to="{ name: 'build', params: { id: build.id } }">
                <span># {{ build.id }}</span>
                <span>{{ build.name }}</span>
                <Badge :text="build.status" :type="buildStatus" />
            </router-link>

            <p><b>Params:</b> {{ getParamsString }}</p>
            <p>
                <b>Tasks:</b> {{ getDoneTasks }}/{{ getTotalTasks }}
                {{ `(${isDone ? "Duration" : "Running for"} ${getDuration})` }}
            </p>
            <p><b>Timestamp:</b> {{ getTimestamp }}</p>
            <router-link :to="{ name: 'build', params: { id: build.id } }" class="btn btn-secondary small float-right" data-cy="open-build-button">
                Open
            </router-link>
            <button
                :to="{ name: 'build', params: { id: build.id } }"
                v-show="!isDone"
                class="btn btn-danger small float-right ml-3"
                data-cy="open-build-button"
                @click="abort"
            >
                Abort
            </button>
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
                running: "warning",
                finished: "success",
                failed: "danger",
                aborted: "info",
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
                            type: "success",
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
    .feed-head {
        @apply flex items-center gap-2 w-full;
    }
}

@media (max-width: 480px) {
    .cell-name {
        max-width: 15ch;
}
</style>
