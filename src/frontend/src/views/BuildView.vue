<template>
  <div class="container">
    <div class="card">
      <div class="card-header">
        <div class="card-title h5">{{ statusUpdate.name }}</div>
        <div class="card-subtitle text-gray">build #{{ statusUpdate.id }}</div>
        <BuildStatus :status="statusUpdate.status"></BuildStatus>
      </div>
      <div class="card-footer">
        <BuildProgress :done="statusUpdate.done_tasks" :total="statusUpdate.total_tasks"></BuildProgress>
      </div>
    </div>
    <TaskItem v-for="item in job.tasks" :key="item.id" :task="item"></TaskItem>
  </div>
</template>

<script>
import {APIURL} from "@/store/communication";
import axios from "axios";
import BuildStatus from "@/components/BuildStatus";
import BuildProgress from "@/components/BuildProgress";
import TaskItem from "@/components/TaskItem";
import {findInContainer} from "@/store/utils";


export default {
    props: {
        id: {
            required: true,
        },
    },
    components: {BuildStatus, BuildProgress, TaskItem},
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
                .get(APIURL + `/build/${this.id}/log/`)
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
                this.job.tasks[index].logs.push(ev);
            } else {
                console.log("Unable to find task:", ev);
            }
        },
        applyBuildUpdate(ev) {
            this.statusUpdate = Object.assign({}, this.statusUpdate, ev);
        },
    },
    data: function() {
        return {
            name: "",
            job: {},
            statusUpdate: {},
            buildLogSubscription: "build:log:" + this.id,
            buildUpdateSubscription: "build:update:" + this.id,
        };
    },
};
</script>

<style scoped lang="scss">
</style>
