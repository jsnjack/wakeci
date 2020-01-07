<template>
  <section v-show="isVisible">
    <div class="divider" :data-content="getDividerText"></div>
    <div class="columns">
      <div class="column">
        <div class="task-header text-left" :class="getBorderClass">
          <BuildStatus :status="task.status"></BuildStatus>
          <span class="h5">{{ name }}</span>
          <Duration v-show="task.status !== 'pending'" :item="task" class="text-small m-1"></Duration>
        </div>
      </div>
      <div class="column text-right">
        <div class="dropdown dropdown-right text-left">
          <div class="btn-group">
            <button @click="reloadLogs" class="btn btn-sm btn-primary">Reload</button>
            <a class="btn btn-sm dropdown-toggle" tabindex="0">
              <i class="icon icon-caret"></i>
            </a>
            <ul class="menu">
              <li class="divider" data-content="ACTIONS"></li>
              <li class="menu-item">
                <a :href="getLogURL" target="_blank">Open</a>
              </li>
              <li class="menu-item">
                <a href="#" @click="clearLogs">Hide</a>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </div>
    <div class="log-container text-left code">
      <pre v-if="content" class="d-block log-line">{{ content }}</pre>
    </div>
  </section>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import Duration from "@/components/Duration";
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
        isVisible: function() {
            // Show only "main" tasks or tasks that were started. For example,
            // there is no need to show "finished" tasks if build failed because
            // they won't be executed anyway
            if (this.task.kind === "main") {
                return true;
            }
            return !(
                this.task.startedAt && this.task.startedAt.indexOf("0001-") === 0
            );
        },
        getBorderClass() {
            return `border-${this.task.kind}`;
        },
        getLogURL() {
            return `/storage/build/${this.buildID}/task_${this.task.id}.log`;
        },
    },
    methods: {
        reloadLogs() {
            axios
                .get(this.getLogURL)
                .then((response) => {
                    this.$notify({
                        text: "Log file has been reloaded",
                        type: "success",
                        duration: 1000,
                    });
                    this.content = response.data;
                    if (this.follow) {
                        this.$nextTick(() => {
                            this.$el.scrollIntoView(false);
                        });
                    }
                })
                .catch((error) => {});
        },
        addLog(log) {
            this.content = this.content + log.data;
            if (this.follow) {
                this.$nextTick(() => {
                    this.$el.scrollIntoView(false);
                });
            }
        },
        clearLogs() {
            this.content = "";
        },
    },
    data: function() {
        return {
            content: "",
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

@media (max-width: 600px) {
  .log-container {
    font-size: 60%;
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
