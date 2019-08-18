<template>
  <section v-show="isVisible">
    <div class="divider" :data-content="getDividerText"></div>
    <div class="columns">
      <div class="column">
        <div class="task-header text-left" :class="getBorderClass">
          <span class="h5">{{ name }}</span>
          <BuildStatus :status="task.status"></BuildStatus>
          <Duration v-show="task.status !== 'pending'" :item="task" class="text-small"></Duration>
        </div>
      </div>
      <div class="column text-right">
        <button @click="reloadLogs" class="btn btn-sm btn-primary">Reload logs</button>
      </div>
    </div>
    <div class="log-container text-left code">
      <pre v-for="item in sortedLogs" :key="item.id" class="d-block log-line">{{ item.data }}</pre>
    </div>
  </section>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import Duration from "@/components/Duration";
import {findInContainer} from "@/store/utils";
import axios from "axios";

export default {
    props: {
        buildID: {
            required: true,
        },
        task: {
            required: true,
        },
        name: {
            required: true,
        },
        follow: {
            required: true,
        },
    },
    components: {BuildStatus, Duration},
    mounted() {
        this.$on("new:log", this.addLog);
    },
    destroyed() {
        this.$off(this.addLog);
    },
    computed: {
        getDividerText: function() {
            return `task #${this.task.id}`;
        },
        sortedLogs: function() {
            return [...this.logItems].sort((a, b) => a.id > b.id);
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
        getBorderClass() {
            return `border-${this.task.kind}`;
        },
    },
    methods: {
        reloadLogs() {
            axios
                .get(`/storage/build/${this.buildID}/task_${this.task.id}.log`)
                .then((response) => {
                    this.$notify({
                        text: "Reloading logs...",
                        type: "success",
                        duration: 1000,
                    });
                    response.data.split("\n").forEach((element, index) => {
                        this.addLog({
                            "id": index,
                            "data": element,
                        });
                    });
                })
                .catch((error) => {});
        },
        addLog(log) {
            const index = findInContainer(this.logItems, "id", log.id);
            if (index[0] === undefined) {
                this.logItems.push(log);
                if (this.follow) {
                    this.$nextTick(() => {
                        this.$el.scrollIntoView(false);
                    });
                }
            }
        },
    },
    data: function() {
        return {
            logItems: [],
        };
    },
};
</script>

<style lang="scss" scoped>
@import "@/assets/colors.scss";

.log-container {
  background: $bg-color;
  margin-left: 1em;
  overflow: auto;
  font-size: 90%;
  pre {
    padding-left: 1em;
    margin: 0;
  }
}

.log-line {
    white-space: pre-wrap;
    word-break: break-word;
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
  border-left: 0.2em solid;
  padding-left: 0.4em;
}
.border-main {
    border-left-color: $primary-color;
}
.border-pending {
    border-left-color: $gray-color;
}
.border-running {
    border-left-color: $warning-color;
}
.border-aborted {
    border-left-color: $secondary-color;
}
.border-failed {
    border-left-color: $error-color;
}
.border-finished {
    border-left-color: $success-color;
}
</style>
