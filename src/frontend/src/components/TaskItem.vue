<template>
  <section v-show="isVisible">
    <div class="divider" :data-content="getDividerText"></div>
    <div class="columns">
      <div class="column">
        <div class="text-left">
          <BuildStatus :status="task.status"></BuildStatus>
          <span class="h5 task-name" @click="reloadLogs">{{ name }}</span>
          <Duration v-show="task.status !== 'pending'" :item="task" class="text-small m-1"></Duration>
        </div>
      </div>
      <div class="column text-right">
        <div class="dropdown dropdown-right text-left">
          <div class="btn-group">
            <button data-cy="reload" @click="reloadLogs" class="btn btn-sm btn-primary">Reload</button>
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
    <div class="log-container text-left code status-border" :class="getBorderClass">
      <pre v-if="content" class="d-block log-line">{{ content }}</pre>
    </div>
  </section>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import Duration from "@/components/Duration";
import axios from "axios";

const FlushContentPeriod = 500;

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
        this.onStatusChange(this.task.status);
    },
    destroyed() {
        this.$off(this.addLog);
    },
    beforeDestroy: function() {
        clearInterval(this.flushInterval);
    },
    watch: {
        "task.status": "onStatusChange",
    },
    computed: {
        getDividerText: function() {
            return `task #${this.task.id}`;
        },
        isVisible: function() {
            // Show only "main" and "finally" tasks or tasks that were started. For example,
            // there is no need to show "finished" tasks if build failed because
            // they won't be executed anyway
            if (this.task.kind === "main" || this.task.kind === "finally") {
                return true;
            }
            return !(
                this.task.startedAt && this.task.startedAt.indexOf("0001-") === 0
            );
        },
        getBorderClass() {
            return `border-${this.task.status}`;
        },
        getLogURL() {
            return `/storage/build/${this.buildID}/task_${this.task.id}.log`;
        },
        getFlushURL() {
            return `/api/build/${this.buildID}/flush`;
        },
    },
    methods: {
        flushLogs() {
            axios
                .post(this.getFlushURL)
                .then((response) => {
                    this._reloadLogs();
                })
                .catch((error) => {});
        },
        _reloadLogs() {
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
        reloadLogs() {
            if (this.task.status === "running") {
                this.flushLogs();
            } else {
                this._reloadLogs();
            }
        },
        addLog(log) {
            // It is better not to add logs directly as it may cause browser
            // to render changes to often
            this.cachedContent = this.cachedContent + log.data;
        },
        flushContent() {
            if (this.cachedContent.length > 0) {
                this.content = this.content + this.cachedContent;
                this.cachedContent = "";
                if (this.follow) {
                    this.$nextTick(() => {
                        this.$el.scrollIntoView(false);
                    });
                }
            }
        },
        clearLogs() {
            this.cachedContent = "";
            this.content = "";
        },
        onStatusChange(value) {
            if (value === "running") {
                this.flushInterval = setInterval(
                    function() {
                        this.flushContent();
                    }.bind(this),
                    FlushContentPeriod
                );
            } else {
                clearInterval(this.flushInterval);
                this.flushContent();
            }
        },
    },
    data: function() {
        return {
            cachedContent: "",
            content: "",
            flushInterval: null,
        };
    },
};
</script>

<style lang="scss" scoped>
@import "@/assets/colors.scss";

.log-container {
  background: $bg-color;
  margin-left: 0.4em;
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

.status-border {
    border-left: 0.25em solid;
}

.border-pending {
  border-left-color: $gray-color;
}
.border-running {
  border-left-color: $warning-color;
}
.border-aborted {
  border-left-color: $primary-color;
}
.border-failed {
  border-left-color: $error-color;
}
.border-finished {
  border-left-color: $success-color;
}

.task-name {
    cursor: pointer;
}
</style>
