<template>
  <section>
    <div class="divider" :data-content="getDividerText"></div>
    <div class="columns">
      <div class="column">
        <h5 class="text-left">{{ task.name }}</h5>
      </div>
      <div class="column text-right">
        <button @click="reloadLogs" class="btn btn-sm btn-primary">Reload logs</button>
        <BuildStatus :status="status"></BuildStatus>
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

export default {
    props: {
        buildID: {
            required: true,
        },
        task: {
            required: true,
        },
        status: {
            required: true,
        },
    },
    components: {BuildStatus},
    computed: {
        getDividerText: function() {
            return `task #${this.task.id}`;
        },
        sortedLogs: function() {
            if (!this.task.logs) {
                return this.task.logs;
            }
            return [...this.task.logs].sort((a, b) => a.id > b.id);
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
