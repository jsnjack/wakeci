<template>
  <div class="container">
    <div class="card build-header">
      <div class="card-header">
        <div class="card-title h5">{{ statusUpdate.name }}</div>
        <div class="card-subtitle text-gray">build #{{ statusUpdate.id }}</div>
        <BuildStatus :status="statusUpdate.status"></BuildStatus>
      </div>
      <div class="card-footer">
        <BuildProgress :done="getDoneTasks" :total="getTotalTasks"></BuildProgress>
      </div>
    </div>
    <div class="columns">
      <ParamItem v-for="(item, index) in statusUpdate.params" :key="index+'param'" :param="item"></ParamItem>
    </div>
    <TaskItem
      v-for="item in job.tasks"
      :key="item.id"
      :task="item"
      :buildID="id"
      :status="getTaskStatus(item.id)"
    ></TaskItem>
  </div>
</template>

<script>
import {APIURL} from "@/store/communication";
import axios from "axios";
import BuildStatus from "@/components/BuildStatus";
import ParamItem from "@/components/ParamItem";
import BuildProgress from "@/components/BuildProgress";
import TaskItem from "@/components/TaskItem";
import {findInContainer} from "@/store/utils";

export default {
    props: {
        id: {
            required: true,
        },
    },
    components: {BuildStatus, BuildProgress, TaskItem, ParamItem},
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
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
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
                }
            } else {
                console.log("Unable to find task:", ev);
            }
        },
        applyBuildUpdate(ev) {
            this.statusUpdate = Object.assign({}, this.statusUpdate, ev);
        },
        getTaskStatus(id) {
            for (const item of this.statusUpdate.tasks) {
                if (item.id === id) {
                    return item.status;
                }
            }
            console.warn("Unknown task", id);
            return "pending";
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
    },
    data: function() {
        return {
            name: "",
            job: {},
            statusUpdate: {
                tasks: [],
            },
            buildLogSubscription: "build:log:" + this.id,
            buildUpdateSubscription: "build:update:" + this.id,
        };
    },
};
</script>

<style scoped lang="scss">
.build-header {
  margin-bottom: 1em;
}
</style>
