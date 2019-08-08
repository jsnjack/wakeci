<template>
  <tr>
    <td>
      <router-link :to="{ name: 'build', params: { id: build.id}}">{{ build.id }}</router-link>
    </td>
    <td>{{ build.name }}</td>
    <td class="hide-xs hide-sm">
        {{ getParamsText }}
    </td>
    <td class="tooltip tooltip-right hide-xs hide-sm" :data-tooltip="getProgressTooltip">
      <BuildProgress :done="getDoneTasks" :total="getTotalTasks"/>
    </td>
    <td>
      <BuildStatus :status="build.status"></BuildStatus>
    </td>
    <td class="hide-xs">
        <Duration v-show="build.status !== 'pending'" :item="build" class="chip"></Duration>
    </td>
    <td class="actions">
      <router-link :to="{ name: 'build', params: { id: build.id}}" class="btn btn-primary item-action">Open</router-link>
      <RunJobButton :params="build.params" :buttonTitle="'Rerun'" :jobName="build.name" class="item-action"></RunJobButton>
      <a v-if="!isDone" :href="getAbortURL" @click.prevent="abort" class="btn btn-error item-action">Abort</a>
    </td>
  </tr>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import BuildProgress from "@/components/BuildProgress";
import RunJobButton from "@/components/RunJobButton";
import Duration from "@/components/Duration";
import axios from "axios";
import {APIURL} from "@/store/communication";

export default {
    components: {BuildStatus, BuildProgress, Duration, RunJobButton},
    props: {
        build: {
            type: Object,
            required: true,
        },
    },
    computed: {
        getProgressTooltip() {
            return `${this.getDoneTasks} of ${this.getTotalTasks}`;
        },
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
        getAbortURL: function() {
            return `${APIURL}/build/${this.build.id}/abort`;
        },
        isDone() {
            switch (this.build.status) {
            case "failed":
            case "finished":
            case "aborted":
                return true;
            }
            return false;
        },
        getParamsText() {
            if (this.build.params) {
                return this.build.params.map((v) => v[Object.keys(v)[0]]).join(", ").substring(0, 50);
            }
            return "";
        },
    },
    methods: {
        abort(event) {
            axios.post(event.target.href)
                .then((response) => {
                    this.$notify({
                        text: `${this.build.id} has been aborted`,
                        type: "success",
                    });
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.item-action {
    margin: 0.25em;
}
</style>
