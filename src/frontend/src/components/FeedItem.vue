<template>
  <tr>
    <td>
      <router-link :to="{ name: 'build', params: { id: build.id}}">{{ build.id }}</router-link>
    </td>
    <td>{{ build.name }}</td>
    <td>
        {{ getParamsText }}
    </td>
    <td class="tooltip tooltip-right" :data-tooltip="getProgressTooltip">
      <BuildProgress :done="getDoneTasks" :total="getTotalTasks"/>
    </td>
    <td>
      <BuildStatus :status="build.status"></BuildStatus>
    </td>
    <td>
        {{ durationText }}
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
import axios from "axios";
import {APIURL} from "@/store/communication";
import {runningDuration, doneDuration} from "@/time";

const updateDurationPeriod = 10000;

export default {
    components: {BuildStatus, BuildProgress},
    props: {
        build: {
            type: Object,
            required: true,
        },
    },
    mounted() {
        this.onStatusChange();
    },
    beforeDestroy: function() {
        clearInterval(this.updateInterval);
    },
    watch: {
        "build.status": "onStatusChange",
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
                    this.$notify({
                        text: `${this.build.id} has been aborted`,
                        type: "success",
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
            if (this.build.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                this.durationText = "";
                return;
            }
            if (this.build.startedAt && !this.build.duration) {
                this.durationText = runningDuration(this.build.startedAt);
                return;
            }
            if (this.build.duration > 0) {
                this.durationText = doneDuration(this.build.duration);
                return;
            }
            return "";
        },
        onStatusChange() {
            if (this.isDone) {
                clearInterval(this.updateInterval);
            } else if (this.build.status === "running" && !this.updateInterval) {
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

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.item-actions a {
    margin-left: 0.25em;
    margin-right: 0.25em;
}
</style>
