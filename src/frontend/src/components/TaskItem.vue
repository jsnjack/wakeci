<template>
  <section v-show="isVisible">
    <div class="divider" :data-content="getDividerText"></div>
    <div class="columns">
      <div class="column">
        <div class="task-header text-left">
          <span class="h5">{{ name }}</span>
          <BuildStatus :status="task.status"></BuildStatus>
          <Duration v-show="task.status !== 'pending'" :item="task" class="chip text-small"></Duration>
        </div>
      </div>
      <div class="column text-right">
        <button @click="reloadLogs" class="btn btn-sm btn-primary">Reload logs</button>
      </div>
    </div>
    <div class="log-container text-left code">
      <pre v-for="item in sortedLogs" :key="item.id" class="d-block">{{ item.data }}</pre>
    </div>
  </section>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import Duration from "@/components/Duration";
import {APIURL} from "@/store/communication";
import axios from "axios";

export default {
    props: {
        buildID: {
            required: true,
        },
        task: {
            required: true,
        },
        logs: {
            required: true,
        },
        name: {
            required: true,
        },
    },
    components: {BuildStatus, Duration},
    computed: {
        getDividerText: function() {
            return `task #${this.task.id}`;
        },
        sortedLogs: function() {
            if (!this.logs) {
                return this.logs;
            }
            return [...this.logs].sort((a, b) => a.id > b.id);
        },
        isVisible: function() {
            // Show only "main" tasks or tasks that were started. For example,
            // there is no need to show "finished" tasks if build failed because
            // they won't be executed anyway
            if (this.task.kind === "main") {
                return true;
            }
            return !(this.task.startedAt && this.task.startedAt.indexOf("0001-") === 0);
        },
    },
    methods: {
        reloadLogs() {
            axios
                .get(APIURL + `/build/${this.buildID}/log/${this.task.id}/`)
                .then((response) => {
                    this.$notify({
                        text: "Reloading logs...",
                        type: "success",
                        duration: 1000,
                    });
                })
                .catch((error) => {});
        },
    },
};
</script>

<style lang="scss" scoped>
@import "@/assets/colors.scss";

.log-container {
  background: $bg-color;
  margin-left: 1em;
  overflow: auto;
  pre {
    padding-left: 1em;
    margin: 0;
  }
}

section {
  margin-top: 2em;
  margin-bottom: 2em;
}

button {
  margin-left: 1em;
  margin-right: 1em;
}

.task-header {
  border-left: 0.2em solid $primary-color;
  padding-left: 0.4em;
}
</style>
