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
        <Duration v-show="build.status !== 'pending'" :item="build" class="chip"></Duration>
    </td>
    <td class="item-actions">
      <router-link :to="{ name: 'build', params: { id: build.id}}" class="btn btn-primary">Open</router-link>
      <a href="#" @click.prevent="toggleRetryModal" class="btn btn-success">Retry</a>
      <a v-if="!isDone" :href="getAbortURL" @click.prevent="abort" class="btn btn-error">Abort</a>

      <div class="modal" v-bind:class="{active: retryModalOpen}">
        <a href="#" @click.prevent="toggleRetryModal" class="modal-overlay" aria-label="Close"></a>
        <div class="modal-container">
          <div class="modal-header">
            <a
              href="#"
              @click.prevent="toggleRetryModal"
              class="btn btn-clear float-right"
              aria-label="Close"
            ></a>
            <div class="modal-title text-uppercase">{{ getRetryModalTitle }}</div>
          </div>
          <div class="modal-body">
            <div class="content">
              <form v-show="this.build.params" ref="form">
                <RunFormItem v-for="item in build.params" :key="item.name" :params="item"></RunFormItem>
              </form>
              <div class="empty" v-show="!this.build.params">
                <p class="empty-title h6 text-uppercase">Empty</p>
              </div>
            </div>
          </div>
          <div class="modal-footer">
            <a href="#" @click.prevent="retry" class="btn btn-primary float-right">Add to queue</a>
          </div>
        </div>
      </div>

    </td>
  </tr>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import BuildProgress from "@/components/BuildProgress";
import RunFormItem from "@/components/RunFormItem";
import Duration from "@/components/Duration";
import axios from "axios";
import {APIURL} from "@/store/communication";

export default {
    components: {BuildStatus, BuildProgress, Duration, RunFormItem},
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
        isRetryModalOpen: function() {
            return this.retryModalOpen;
        },
        getRetryModalTitle: function() {
            return `${this.build.name} job parameters`;
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
                .catch((error) => {});
        },
        retry(event) {
            this.toggleRetryModal();
            const url =
        `${APIURL}/job/${this.build.name}/run?` +
        new URLSearchParams(
            Array.from(new FormData(this.$refs.form))
        ).toString();
            axios
                .post(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.build.name} has been scheduled (#${response.data})`,
                        type: "success",
                    });
                })
                .catch((error) => {});
        },
        toggleRetryModal(event) {
            this.retryModalOpen = !this.retryModalOpen;
        },
    },
    data: function() {
        return {
            retryModalOpen: false,
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
