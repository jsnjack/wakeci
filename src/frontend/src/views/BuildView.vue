<template>
  <div class="container">
    <div class="card build-header">
      <div class="card-header">
        <div class="card-title h5">{{ statusUpdate.name }} #{{ statusUpdate.id }}</div>
        <div class="card-subtitle text-gray">{{ job.desc }}</div>
        <BuildStatus :status="statusUpdate.status"></BuildStatus>
        <Duration v-show="statusUpdate.status !== 'pending'" :item="statusUpdate" class="chip"></Duration>
      </div>
      <div class="card-footer">
        <BuildProgress :done="getDoneTasks" :total="getTotalTasks"></BuildProgress>
      </div>
    </div>
    <div class="columns">
      <ParamItem v-for="(item, index) in statusUpdate.params" :key="index+'param'" :param="item"></ParamItem>
    </div>
    <TaskItem
      v-for="item in statusUpdate.tasks"
      :key="item.id"
      :task="item"
      :buildID="id"
      :logs="job.tasks[item.id].logs"
      :name="job.tasks[item.id].name"
      :ref="'task-'+item.id"
    ></TaskItem>
    <Artifacts :artifacts="getArtifacts" :buildID="statusUpdate.id"></Artifacts>
    <div class="form-group float-right">
      <label class="form-switch">
        <input type="checkbox" v-model="follow" />
        <i class="form-icon"></i> Follow logs
      </label>
    </div>
  </div>
</template>

<script>
import {APIURL} from "@/store/communication";
import axios from "axios";
import BuildStatus from "@/components/BuildStatus";
import Duration from "@/components/Duration";
import ParamItem from "@/components/ParamItem";
import BuildProgress from "@/components/BuildProgress";
import TaskItem from "@/components/TaskItem";
import Artifacts from "@/components/Artifacts";
import {findInContainer} from "@/store/utils";

export default {
    props: {
        id: {
            required: true,
        },
    },
    components: {
        BuildStatus,
        BuildProgress,
        TaskItem,
        ParamItem,
        Artifacts,
        Duration,
    },
    mounted() {
        this.fetch();
        this.subscribe();
    },
    destroyed() {
        this.unsubscribe();
    },
    methods: {
        subscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:subscribe",
                data: {
                    to: [this.buildLogSubscription, this.buildUpdateSubscription],
                },
            });
            this.$eventHub.$on(this.buildLogSubscription, this.applyBuildLog);
            this.$eventHub.$on(this.buildUpdateSubscription, this.applyBuildUpdate);
        },
        unsubscribe() {
            this.$store.commit("WS_SEND", {
                type: "in:unsubscribe",
                data: {
                    to: [this.buildLogSubscription, this.buildUpdateSubscription],
                },
            });
            this.$eventHub.$off(this.buildLogSubscription);
            this.$eventHub.$off(this.buildUpdateSubscription);
        },
        fetch() {
            axios
                .get(APIURL + `/build/${this.id}/`)
                .then((response) => {
                    this.statusUpdate = response.data.status_update;
                    this.job = response.data.job;
                })
                .catch((error) => {});
        },
        applyBuildLog(ev) {
            const index = findInContainer(this.job.tasks, "id", ev.task_id)[1];
            if (index !== undefined) {
                if (this.job.tasks[index].logs === null) {
                    this.job.tasks[index].logs = [];
                }
                const logIndex = findInContainer(
                    this.job.tasks[index].logs,
                    "id",
                    ev.id
                );
                if (logIndex[0] === undefined) {
                    this.job.tasks[index].logs.push(ev);
                    if (this.follow) {
                        this.$nextTick(() => {
                            this.$refs["task-" + index][0].$el.scrollIntoView(false);
                        });
                    }
                }
            } else {
                console.log("Unable to find task:", ev);
            }
        },
        applyBuildUpdate(ev) {
            this.statusUpdate = Object.assign({}, this.statusUpdate, ev);
        },
    },
    computed: {
        getProgressTooltip() {
            return `${this.getDoneTasks} of ${this.getTotalTasks}`;
        },
        getDoneTasks() {
            return this.statusUpdate.tasks.filter((item) => {
                return item.status !== "pending" && item.status !== "running";
            }).length;
        },
        getTotalTasks() {
            return this.statusUpdate.tasks.length;
        },
        getArtifacts() {
            return this.statusUpdate.artifacts || [];
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
};
</script>

<style scoped lang="scss">
.build-header {
  margin-bottom: 1em;
}
</style>
