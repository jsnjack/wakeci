<template>
  <section>
    <div class="columns">
      <div class="column">
        <h5 class="text-left">{{ task.name }}</h5>
      </div>
      <div class="column">
        <button @click="reloadLogs" class="btn btn-sm btn-primary">Reload logs</button>
      </div>
      <div class="column">
          <BuildStatus :status="task.status"></BuildStatus>
      </div>
    </div>
    <div class="log-container text-left">
      <span v-for="item in task.logs" :key="item.id" class="d-block">{{ item.data }}</span>
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
    },
    components: {BuildStatus},
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
                        text: error,
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
}
</style>
