<template>
  <section>
    <div class="divider" :data-content="getDividerText"></div>
    <div class="columns">
      <div class="column">
        <h5 class="text-left">{{ task.name }}</h5>
      </div>
      <div class="column text-right">
        <button @click="reloadLogs" class="btn btn-sm btn-primary">Reload logs</button>
        <BuildStatus :status="task.status"></BuildStatus>
      </div>
    </div>
    <div class="log-container text-left code">
      <pre v-for="item in sortedLogs" :key="item.id" class="d-block">{{ item.data }}</pre>
    </div>
  </section>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import {APIURL} from "@/store/communication";
import axios from "axios";
import {runningDuration, doneDuration, updateDurationPeriod} from "@/duration";


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
    },
    components: {BuildStatus},
    mounted() {
        this.onStatusChange();
    },
    beforeDestroy: function() {
        clearInterval(this.updateInterval);
    },
    watch: {
        "task.status": "onStatusChange",
    },
    computed: {
        getDividerText: function() {
            return `task #${this.task.id} - ${this.durationText}`;
        },
        sortedLogs: function() {
            if (!this.logs) {
                return this.logs;
            }
            return [...this.logs].sort((a, b) => a.id > b.id);
        },
        isDone() {
            switch (this.task.status) {
            case "failed":
            case "finished":
            case "aborted":
                return true;
            }
            return false;
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
                .catch((error) => {
                    this.$notify({
                        text: error.response && error.response.data || error,
                        type: "error",
                    });
                });
        },
        updateDuration() {
            if (this.task.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                this.durationText = "";
                return;
            }
            if (this.task.status === "running") {
                this.durationText = runningDuration(this.task.startedAt);
                return;
            }
            if (this.task.duration > 0) {
                this.durationText = doneDuration(this.task.duration);
                return;
            }
            return "";
        },
        onStatusChange() {
            if (this.isDone) {
                clearInterval(this.updateInterval);
            } else if (this.task.status === "running" && !this.updateInterval) {
                this.updateInterval = setInterval(function() {
                    this.updateDuration();
                }.bind(this), updateDurationPeriod);
            }
            this.updateDuration();
        },
    },
    data: function() {
        return {
            updateInterval: null,
            durationText: "",
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

h5{
    border-left: 0.2em solid $primary-color;
    padding-left: 0.4em;
}

</style>
