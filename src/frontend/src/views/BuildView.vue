<template>
    <div class="build-view">
        <Card class="build-header">
            <div class="title-holder">
                <b>Tasks:</b>
                <div class="tasks-status">
                    <TaskStatus
                        v-for="task in statusUpdate.tasks"
                        :key="task.id"
                        :status="task.status"
                        :task-title="job.tasks[task.id].name"
                        @click="openAndScrollToTask(task.id)"
                        showLabel
                        clickable
                    />
                </div>
            </div>
            <div class="build-actions">
                <div class="follow-holder">
                    <Toggle v-model="follow" />
                    Follow
                </div>

                <button class="btn btn-success small" @click="showRunModal = true">Rerun</button>

                <ArtifactItem
                    v-if="getArtifacts.length"
                    :artifacts="getArtifacts"
                    :build-i-d="statusUpdate.id"
                    title="Artifacts"
                />
            </div>
        </Card>

        <FeedItem :build="statusUpdate" :showOpen="false" :job="job" />

        <br />

        <TaskItem
            v-for="item in statusUpdate.tasks"
            :key="item.id"
            :ref="'task-' + item.id"
            :task="item"
            :build-i-d="id"
            :build-status="statusUpdate.status"
            :name="job.tasks[item.id].name"
            :follow="follow"
        />

        <RunJobModal v-show="showRunModal" @close="showRunModal = false" :params="statusUpdate.params" :job-name="statusUpdate.name" />
    </div>
</template>

<script>
import vuex from 'vuex';
import axios from 'axios';
import DurationElement from '@/components/DurationElement.vue';
import TaskItem from '@/components/TaskItem.vue';
import ArtifactItem from '@/components/ArtifactItem.vue';
import FeedItem from '@/components/FeedItem.vue';
import RunJobModal from '@/components/RunJobModal.vue';
import Toggle from '@/components/ui/Toggle.vue';
import Card from '@/components/ui/Card.vue';
import TaskStatus from '@/components/TaskStatus.vue';
import MoreOptions from '@/components/ui/MoreOptions.vue';

export default {
    components: {
        TaskItem,
        ArtifactItem,
        DurationElement,
        RunJobModal,
        FeedItem,
        Toggle,
        Card,
        TaskStatus,
        MoreOptions,
    },
    props: {
        id: {
            type: Number,
            required: true,
        },
    },
    data: function () {
        return {
            name: "",
            job: {},
            statusUpdate: {
                tasks: [],
                id: NaN,
            },
            buildLogSubscription: "build:log:" + this.id,
            buildUpdateSubscription: "build:update:" + this.id,
            follow: true,
            showRunModal: false,
        };
    },
    computed: {
        ...vuex.mapState(["ws"]),
        getMainTasks() {
            return this.statusUpdate.tasks.filter((item) => {
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
        getArtifacts() {
            if (this.statusUpdate.build_artifacts) {
                return this.statusUpdate.build_artifacts;
            }

            if (this.statusUpdate.artifacts) {
                // Deprecate
                const data = [];
                this.statusUpdate.artifacts.forEach((element) => {
                    data.push({ filename: element });
                });
                return data;
            }
            return [];
        },
        getAbortURL: function () {
            return `/api/build/${this.id}/abort`;
        },
        isDone() {
            switch (this.statusUpdate.status) {
                case "failed":
                case "finished":
                case "aborted":
                case "timed out":
                case "skipped":
                    return true;
            }
            return false;
        },
    },
    watch: {
        "ws.connected": "onWSChange",
    },
    mounted() {
        document.title = `#${this.id} - wakeci`;
        this.fetch();
        this.subscribe();
        this.emitter.on(this.buildUpdateSubscription, this.applyBuildUpdate);
    },
    unmounted() {
        this.unsubscribe();
        this.emitter.off(this.buildUpdateSubscription, this.applyBuildUpdate);
    },
    methods: {
        subscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:subscribe",
                data: {
                    to: [this.buildLogSubscription, this.buildUpdateSubscription],
                },
            });
        },
        unsubscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:unsubscribe",
                data: {
                    to: [this.buildLogSubscription, this.buildUpdateSubscription],
                },
            });
        },
        fetch() {
            axios
                .get(`/api/build/${this.id}`)
                .then((response) => {
                    this.statusUpdate = response.data.status_update;
                    this.job = response.data.job;
                    this.updateTitle();
                })
                .catch((error) => {});
        },
        abort(event) {
            axios
                .post(event.target.href)
                .then((response) => {
                    this.$notify({
                        text: `${this.id} has been aborted`,
                        type: "success",
                    });
                })
                .catch((error) => {});
        },
        applyBuildUpdate(ev) {
            this.statusUpdate = Object.assign({}, this.statusUpdate, ev);
            this.updateTitle();
        },
        updateTitle() {
            document.title = `#${this.id} - ${this.statusUpdate.status} - wakeci`;
        },
        onWSChange(value) {
            if (value) {
                this.subscribe();
            } else {
                this.unsubscribe();
            }
        },
        openAndScrollToTask(id) {
            console.log(this.$refs[`task-${id}`][0]);
            this.$refs[`task-${id}`][0].reloadLogs();
            this.$nextTick(() => {
                this.$refs[`task-${id}`][0].$el.scrollIntoView({
                    behavior: "smooth",
                    block: "end",
                });
            });
        },
    },
};
</script>

<style scoped lang="scss">
.build-view {
    z-index: 1;
    .build-header {
        @apply sticky top-0 left-0 z-20 shadow flex justify-between items-center gap-4 mb-6 transform -translate-x-6 -translate-y-6 px-6 rounded-none flex-wrap;
        width: calc(100% + 3rem);
        .follow-holder {
            @apply flex items-center gap-1;
        }
        .title-holder {
            @apply max-w-full overflow-x-clip;
            .tasks-status {
                @apply flex-1 grid grid-flow-col auto-cols-max items-center justify-start w-max max-w-full gap-8 overflow-x-auto p-2 pb-3;
            }
        }
        .build-actions {
            @apply flex gap-4 ml-auto;
        }
    }
}
</style>
