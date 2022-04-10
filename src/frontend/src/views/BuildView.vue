<template>
  <div class="container grid-xl">
    <div class="card build-header">
      <div class="card-header">
        <div class="card-title h5">
          {{ statusUpdate.name }} #{{ statusUpdate.id }}
        </div>
        <div class="card-subtitle text-gray">
          {{ job.desc }}
        </div>
        <BuildStatus :status="statusUpdate.status" />
        <DurationElement
          v-show="statusUpdate.status !== 'pending'"
          :item="statusUpdate"
        />
        <div class="float-right">
          <a
            v-if="!isDone"
            :href="getAbortURL"
            class="btn btn-error item-action"
            data-cy="abort-build-button"
            @click.prevent="abort"
          >Abort</a>
          <RunJobButton
            :params="statusUpdate.params"
            :button-title="'Rerun'"
            :job-name="statusUpdate.name"
            class="item-action"
          />
        </div>
      </div>
      <div class="card-footer">
        <BuildProgress
          v-if="!statusUpdate.eta"
          :done="getDoneTasks"
          :total="getTotalTasks"
        />
        <BuildProgressETA
          v-if="statusUpdate.eta"
          :eta="statusUpdate.eta"
          :started-at="statusUpdate.startedAt"
          :build-duration="statusUpdate.duration"
        />
      </div>
    </div>
    <div class="columns">
      <ParamItem
        v-for="(item, index) in statusUpdate.params"
        :key="index+'param'"
        :param="item"
      />
    </div>
    <TaskItem
      v-for="item in statusUpdate.tasks"
      :key="item.id"
      :ref="'task-'+item.id"
      :task="item"
      :build-i-d="id"
      :build-status="statusUpdate.status"
      :name="job.tasks[item.id].name"
      :follow="follow"
    />
    <ArtifactItem
      :artifacts="getArtifacts"
      :build-i-d="statusUpdate.id"
    />
    <div class="follow-logs form-group float-right label">
      <label class="form-switch">
        <input
          v-model="follow"
          type="checkbox"
        >
        <i class="form-icon" /> Follow
      </label>
    </div>
  </div>
</template>

<script>
import vuex from "vuex";
import axios from "axios";
import BuildStatus from "@/components/BuildStatus";
import DurationElement from "@/components/DurationElement";
import ParamItem from "@/components/ParamItem";
import BuildProgress from "@/components/BuildProgress";
import BuildProgressETA from "@/components/BuildProgressETA";
import RunJobButton from "@/components/RunJobButton";
import TaskItem from "@/components/TaskItem";
import ArtifactItem from "@/components/ArtifactItem";

export default {
    components: {
        BuildStatus,
        BuildProgress,
        BuildProgressETA,
        TaskItem,
        ParamItem,
        ArtifactItem,
        DurationElement,
        RunJobButton,
    },
    props: {
        id: {
            type: Number,
            required: true,
        },
    },
    data: function() {
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

            if (this.statusUpdate.artifacts) { // Deprecate
                const data = [];
                this.statusUpdate.artifacts.forEach((element) => {
                    data.push({"filename": element});
                });
                return data;
            }
            return [];
        },
        getAbortURL: function() {
            return `/api/build/${this.id}/abort`;
        },
        isDone() {
            switch (this.statusUpdate.status) {
            case "failed":
            case "finished":
            case "aborted":
            case "timed out":
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
    },
};
</script>

<style scoped lang="scss">
.build-header {
  margin-bottom: 1em;
}
summary:hover {
  cursor: pointer;
}
.item-action {
    margin: 0.25em;
}
.follow-logs {
    position: fixed;
    bottom: 10px;
    right: 10px;
    opacity: 0.8;
    border-radius: 10px;
}
</style>
