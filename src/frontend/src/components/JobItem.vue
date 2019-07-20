<template>
  <tr>
    <td>{{ job.name }}</td>
    <td class="item-actions">
      <a @click.prevent="toggle" href="#" class="btn btn-success">Start</a>
      <router-link :to="{ name: 'jobEdit', params: { name: job.name}}" class="btn btn-primary">Edit</router-link>

      <div class="modal" v-bind:class="{active: modalOpen}">
        <a href="#" @click.prevent="toggle" class="modal-overlay" aria-label="Close"></a>
        <div class="modal-container">
          <div class="modal-header">
            <a
              href="#"
              @click.prevent="toggle"
              class="btn btn-clear float-right"
              aria-label="Close"
            ></a>
            <div class="modal-title text-uppercase">{{ getModalTitle }}</div>
          </div>
          <div class="modal-body">
            <div class="content">
              <form v-show="this.job.defaultParams" ref="form">
                <RunFormItem v-for="item in job.defaultParams" :key="item.name" :params="item"></RunFormItem>
              </form>
              <div class="empty" v-show="!this.job.defaultParams">
                <p class="empty-title h6 text-uppercase">Empty</p>
              </div>
            </div>
          </div>
          <div class="modal-footer">
            <a
              href="#"
              @click.prevent="run"
              class="btn btn-primary float-right"
              aria-label="Close"
            >Add to queue</a>
          </div>
        </div>
      </div>
    </td>
  </tr>
</template>

<script>
import axios from "axios";
import {APIURL} from "@/store/communication";
import RunFormItem from "@/components/RunFormItem";

export default {
    props: {
        job: {
            type: Object,
            required: true,
        },
    },
    components: {RunFormItem},
    methods: {
        run(event) {
            this.toggle();
            const url = `${APIURL}/job/${this.job.name}/run?` + new URLSearchParams(Array.from(new FormData(this.$refs.form))).toString();
            axios
                .post(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.job.name} has been scheduled (#${response.data})`,
                        type: "success",
                    });
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
        toggle(event) {
            this.modalOpen = !this.modalOpen;
        },
    },
    computed: {
        isModalOpen: function() {
            return this.modalOpen;
        },
        getModalTitle: function() {
            return `${this.job.name} job parameters`;
        },
    },
    data: function() {
        return {
            modalOpen: false,
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
