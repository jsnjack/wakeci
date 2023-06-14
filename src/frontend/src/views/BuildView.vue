<template>
    <article>
        <div class="row">
            <div class="max medium-padding">
                <div>
                    <h6>{{ statusUpdate.name }}</h6>
                    <p>{{ job.desc }}</p>
                </div>
            </div>
            <div class="medium-padding">
                <div>
                    <div class="row">
                        <BuildStatus :status="statusUpdate.status" />
                        <div>{{ statusUpdate.status }}</div>
                    </div>
                    <div class="small-padding">
                        <SimpleDuration :item="statusUpdate" />
                        <SimpleStartedAgo :item="statusUpdate" />
                    </div>
                </div>
            </div>
        </div>
        <div class="row">
            <div class="max"></div>
            <button
                class="circle transparent"
                @click.prevent="hideAll"
            >
                <i>hide</i>
                <div class="tooltip bottom">Hide all logs</div>
            </button>
            <a
                class="button circle transparent"
                :disabled="isDone"
                @click.prevent="abort"
                :href="getAbortURL"
            >
                <i>stop</i>
                <div class="tooltip bottom">Abort</div>
            </a>
            <RunJobButton
                :params="job.defaultParams"
                :job-name="job.name"
                :icon="'replay'"
            />
        </div>
    </article>

    <article v-if="params && params.length > 0">
        <FullParamItem
            v-for="(item, index) in statusUpdate.params"
            :key="index + 'param'"
            :param="item"
        />
    </article>

    <div>
        <TaskItem
            v-for="item in statusUpdate.tasks"
            :key="item.id"
            :ref="'task-' + item.id"
            :task="item"
            :build-i-d="id"
            :name="job.tasks[item.id].name"
            :follow="follow"
        />
    </div>

    <ArtifactItem
        :artifacts="getArtifacts"
        :build-i-d="statusUpdate.id"
    />

    <label
        style="opacity: 0.8"
        class="switch icon fixed bottom right medium-margin"
    >
        <div class="tooltip left">Follow logs</div>
        <input
            type="checkbox"
            v-model="follow"
        />
        <span>
            <i>route</i>
        </span>
    </label>
</template>

<script>
import vuex from "vuex";
import axios from "axios";
import BuildStatus from "@/components/BuildStatus.vue";
import ParamItem from "@/components/ParamItem.vue";
import FullParamItem from "@/components/FullParamItem.vue";
import BuildProgress from "@/components/BuildProgress.vue";
import BuildProgressETA from "@/components/BuildProgressETA.vue";
import RunJobButton from "@/components/RunJobButton.vue";
import TaskItem from "@/components/TaskItem.vue";
import ArtifactItem from "@/components/ArtifactItem.vue";
import SimpleDuration from "@/components/SimpleDuration.vue";
import SimpleStartedAgo from "@/components/SimpleStartedAgo.vue";

export default {
    components: {
        BuildStatus,
        BuildProgress,
        BuildProgressETA,
        TaskItem,
        ParamItem,
        ArtifactItem,
        RunJobButton,
        SimpleDuration,
        SimpleStartedAgo,
        FullParamItem,
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
        this.$store.commit("SET_CURRENT_PAGE", `#${this.id}`);
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
            this.$store.commit("SET_CURRENT_PAGE", `#${this.id} - ${this.statusUpdate.status}`);
        },
        onWSChange(value) {
            if (value) {
                this.subscribe();
            } else {
                this.unsubscribe();
            }
        },
    },
};
</script>

<style scoped lang="scss"></style>
