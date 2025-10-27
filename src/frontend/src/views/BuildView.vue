<template>
    <NotFound v-if="empty" />
    <article v-if="!empty">
        <div :class="{ row: isDesktop }">
            <div class="max medium-padding">
                <div>
                    <h5>{{ statusUpdate.name }}</h5>
                    <p class="large-text">{{ job.desc }}</p>
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
                @click.prevent="toggleHideAllLogs"
            >
                <i v-if="!hideAllLogs">hide</i>
                <i v-else>system_update_alt</i>
                <div
                    v-if="!hideAllLogs"
                    class="tooltip bottom"
                >
                    Hide all logs (Ctrl + Alt + H)
                </div>
                <div
                    v-else
                    class="tooltip bottom"
                >
                    Stream logs
                </div>
            </button>
            <a
                class="button circle transparent"
                :disabled="isDone ? true : null"
                @click.prevent="abort"
                :href="getAbortURL"
                data-cy="abort-build-button"
            >
                <i>stop</i>
                <div class="tooltip bottom">Abort</div>
            </a>
            <StartBuildNowButton
                v-if="statusUpdate.id"
                :status="statusUpdate.status"
                :build-i-d="statusUpdate.id"
            />
            <RunJobButton
                :params="statusUpdate.params"
                :job-name="job.name"
                :icon="'replay'"
            />
        </div>
    </article>

    <article v-if="statusUpdate.params && statusUpdate.params.length > 0">
        <div class="large-text">Parameters</div>
        <ParamItem
            v-for="(item, index) in statusUpdate.params"
            :key="index + 'param'"
            :param="item"
            :includeKeys="true"
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
            :hideAllLogs="hideAllLogs"
        />
    </div>

    <ArtifactItem
        :artifacts="getArtifacts"
        :build-i-d="statusUpdate.id"
    />

    <label
        v-if="!hideAllLogs"
        style="opacity: 0.8"
        class="switch icon fixed bottom right medium-margin"
    >
        <div class="tooltip left">Toggle following logs (Ctrl + Alt + F)</div>
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
import ArtifactItem from "@/components/ArtifactItem.vue";
import BuildStatus from "@/components/BuildStatus.vue";
import NotFound from "@/components/NotFound.vue";
import ParamItem from "@/components/ParamItem.vue";
import RunJobButton from "@/components/RunJobButton.vue";
import SimpleDuration from "@/components/SimpleDuration.vue";
import SimpleStartedAgo from "@/components/SimpleStartedAgo.vue";
import TaskItem from "@/components/TaskItem.vue";
import axios from "axios";
import vuex from "vuex";
import StartBuildNowButton from "../components/StartBuildNowButton.vue";

export default {
    components: {
        BuildStatus,
        TaskItem,
        ParamItem,
        ArtifactItem,
        RunJobButton,
        SimpleDuration,
        SimpleStartedAgo,
        ParamItem,
        NotFound,
        StartBuildNowButton,
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
            hideAllLogs: false,
            empty: false,
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
        isDesktop() {
            // Info about sizes:
            // https://github.com/beercss/beercss/blob/acd6fe4e5aefd7c24fe4df30aa12e34f9ca92f90/src/cdn/helpers/responsive.css
            return window.innerWidth > 600;
        },
    },
    watch: {
        "ws.connected": "onWSChange",
        hideAllLogs: "onHideAllLogsChange",
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", `#${this.id}`);
        document.addEventListener("keyup", this.onKeyUp);
        this.fetch();
        this.subscribe();
        this.emitter.on(this.buildUpdateSubscription, this.applyBuildUpdate);
    },
    unmounted() {
        document.removeEventListener("keyup", this.onKeyUp);
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
                .catch((error) => {
                    this.empty = true;
                });
        },
        abort(event) {
            axios
                .post(event.target.href)
                .then((response) => {
                    this.$notify({
                        text: `${this.id} has been aborted`,
                        type: "primary",
                    });
                })
                .catch((error) => {});
        },
        applyBuildUpdate(ev) {
            this.statusUpdate = Object.assign({}, this.statusUpdate, ev);
            this.updateTitle();
        },
        updateTitle() {
            const statusIcon = this.getStatusIcon(this.statusUpdate.status);
            this.$store.commit("SET_CURRENT_PAGE", { title: `#${this.id} - ${this.statusUpdate.status}`, icon: statusIcon });
        },
        getStatusIcon(status) {
            switch (status) {
                case "failed":
                    return "❌";
                case "finished":
                    return "✅";
                case "aborted":
                    return "🛑";
                case "timed out":
                    return "⏱️";
                case "skipped":
                    return "⤵️";
                case "pending":
                    return "⏳";
                case "running":
                    return "▶️";
            }
            return "";
        },
        onWSChange(value) {
            if (value) {
                this.subscribe();
            } else {
                this.unsubscribe();
            }
        },
        toggleHideAllLogs() {
            this.hideAllLogs = !this.hideAllLogs;
        },
        onHideAllLogsChange(value) {
            if (value) {
                this.$store.commit("WS_SEND", {
                    type: "in:unsubscribe",
                    data: {
                        to: [this.buildLogSubscription],
                    },
                });
                this.follow = false;
            } else {
                this.$store.commit("WS_SEND", {
                    type: "in:subscribe",
                    data: {
                        to: [this.buildLogSubscription],
                    },
                });
                this.follow = true;
            }
        },
        onKeyUp(event) {
            console.log(event);
            if (event.ctrlKey && event.altKey && event.code === "KeyF") {
                this.follow = !this.follow;
                return;
            }
            if (event.ctrlKey && event.altKey && event.code === "KeyH") {
                this.hideAllLogs = !this.hideAllLogs;
            }
        },
    },
};
</script>

<style scoped ></style>
