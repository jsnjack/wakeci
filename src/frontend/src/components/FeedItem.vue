<template>
    <Card>
        <ProgressBar :type="buildStatus" :progress="getDoneTasks / getTotalTasks" />

        <div class="feed-item-content">
            <router-link class="feed-head" :to="{ name: 'build', params: { id: build.id } }">
                <span># {{ build.id }}</span>
                <span>{{ build.name }}</span>
                <Badge :text="build.status" :type="buildStatus" />
            </router-link>
<<<<<<< HEAD
        </td>
        <td>
            <div class="cell-name">
                {{ build.name }}
            </div>
        </td>
        <td class="hide-xs hide-sm">
            <div
                v-show="build.params"
                class="label param tooltip"
                :data-tooltip="getParamsTooltip"
                data-cy="params-text"
            >
                {{ getParamsText }}
            </div>
        </td>
        <td class="hide-xs hide-sm hide-md">
            <BuildProgress
                v-if="!build.eta"
                :done="getDoneTasks"
                :total="getTotalTasks"
            />
            <BuildProgressETA
                v-if="build.eta"
                :eta="build.eta"
                :started-at="build.startedAt"
                :build-duration="build.duration"
            />
        </td>
        <td>
            <BuildStatus :status="build.status" />
        </td>
        <td class="hide-xs">
            <DurationElement
                v-show="build.status !== 'pending'"
                :item="build"
                :use-global-duration-mode-state="true"
            />
        </td>
        <td class="actions">
            <div class="btn-group">
                <router-link
                    :to="{ name: 'build', params: { id: build.id } }"
                    class="btn btn-primary"
                    data-cy="open-build-button"
                >
                    Open
                </router-link>
                <a
                    v-if="!isDone"
                    :href="getAbortURL"
                    class="btn btn-error btn-action"
                    data-cy="abort-build-button"
                    data-tooltip="Abort build"
                    @click.prevent="abort"
                    ><i class="icon icon-cross"
                /></a>
                <a
                    v-if="build.status === 'pending'"
                    :href="getStartURL"
                    class="btn btn-action tooltip tooltip-bottom"
                    data-cy="start-build-button"
                    data-tooltip="Start now"
                    @click.prevent="start"
                    ><i class="icon icon-forward"
                /></a>
            </div>
        </td>
    </tr>
=======

            <p><b>Params:</b> {{ build.params }}</p>
            <p><b>Tasks:</b> {{ getDoneTasks }}/{{ getTotalTasks }}</p>
            <router-link :to="{ name: 'build', params: { id: build.id } }" class="btn btn-secondary small" data-cy="open-build-button"> Open </router-link>
        </div>
    </Card>
>>>>>>> 2820fe0 (Add partial FeedItem new card)
</template>

<script>
import axios from "axios";
import Card from "./ui/Card.vue";
import Badge from "./ui/Badge.vue";
import ProgressBar from "./ui/ProgressBar.vue";

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
        getAbortURL: function () {
            return `/api/build/${this.build.id}/abort`;
        },
        getStartURL: function () {
            return `/api/build/${this.build.id}/start`;
        },
        buildStatus() {
            const status = {
                running: "warning",
                finished: "success",
                failed: "danger",
                aborted: "danger",
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
        getParamsTooltip() {
            if (this.build.params) {
                return this.build.params.map((v) => v[Object.keys(v)[0]]).join("\n");
            }
            return "";
        },
    },
    methods: {
        abort(event) {
            axios
                .post(event.target.href || event.target.parentElement.href)
                .then((response) => {
                    this.$notify({ text: `${this.build.id} has been aborted`, type: "success" });
                })
                .catch((error) => {});
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
.param {
    margin: 0.25em;
}
.param:hover {
    cursor: default;
}
.cell-name {
    max-width: 20ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
