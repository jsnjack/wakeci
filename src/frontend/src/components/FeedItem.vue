<template>
  <tr>
    <td>
      <router-link :to="{ name: 'build', params: { id: build.id}}">{{ build.id }}</router-link>
    </td>
    <td>
        <div class="cell-name">{{ build.name }}</div>
    </td>
    <td class="hide-xs hide-sm">
        <div class="label param tooltip" :data-tooltip="getParamsTooltip">{{ getParamsText }}</div>
    </td>
    <td class="tooltip tooltip-right hide-xs hide-sm hide-md" :data-tooltip="getProgressTooltip">
      <BuildProgress :done="getDoneTasks" :total="getTotalTasks" />
    </td>
    <td>
      <BuildStatus :status="build.status"></BuildStatus>
    </td>
    <td class="hide-xs">
      <Duration
        v-show="build.status !== 'pending'"
        :item="build"
      ></Duration>
    </td>
    <td class="actions">
      <router-link
        :to="{ name: 'build', params: { id: build.id}}"
        class="btn btn-primary item-action"
      >Open</router-link>
      <a
        v-if="!isDone"
        :href="getAbortURL"
        @click.prevent="abort"
        class="btn btn-error item-action"
      >Abort</a>
    </td>
  </tr>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import BuildProgress from "@/components/BuildProgress";
import Duration from "@/components/Duration";
import axios from "axios";

export default {
    components: {BuildStatus, BuildProgress, Duration},
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
            return `/api/build/${this.build.id}/abort`;
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
                return Object.values(this.build.params[0])[0].substring(0, 20);
            }
            return "";
        },
        getParamsTooltip() {
            if (this.build.params) {
                return this.build.params
                    .map((v) => v[Object.keys(v)[0]])
                    .join("\n");
            }
            return "";
        },
    },
    methods: {
        abort(event) {
            axios
                .post(event.target.href)
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
.param {
  margin: 0.25em;
}
.param:hover{
    cursor: default;
}
.cell-name{
    max-width: 20ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
</style>
