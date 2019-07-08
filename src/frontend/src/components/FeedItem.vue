<template>
  <tr>
    <td>{{ build.name }}</td>
    <td>
      <router-link :to="{ name: 'build', params: { id: build.id}}">{{ build.id }}</router-link>
    </td>
    <td>
        <!-- <div class="columns">
        <ParamItem v-for="(item, index) in build.params" :key="index+'param'" :param="item"></ParamItem>
        </div> -->
        {{ getParamsText }}
    </td>
    <td class="tooltip tooltip-right" :data-tooltip="getProgressTooltip">
      <BuildProgress :done="getDoneTasks" :total="getTotalTasks"/>
    </td>
    <td>
      <BuildStatus :status="build.status"></BuildStatus>
    </td>
    <td class="item-actions">
      <router-link :to="{ name: 'build', params: { id: build.id}}" class="btn btn-primary">Open</router-link>
      <a v-if="!isDone" :href="getAbortURL" @click.prevent="abort" class="btn btn-error">Abort</a>
    </td>
  </tr>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import BuildProgress from "@/components/BuildProgress";
import ParamItem from "@/components/ParamItem";
import axios from "axios";
import {APIURL} from "@/store/communication";

export default {
    components: {BuildStatus, BuildProgress, ParamItem},
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
        getDoneTasks() {
            return this.build.tasks.filter((item) => {
                return item.status !== "pending" && item.status !== "running";
            }).length;
        },
        getTotalTasks() {
            return this.build.tasks.length;
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
                    console.log(response);
                    this.$notify({
                        text: `${this.build.id} has been aborted`,
                        type: "success",
                    });
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.item-actions a {
    margin-left: 0.25em;
    margin-right: 0.25em;
}
</style>
