<template>
  <tr>
    <td>
        <div>{{ job.name }}</div>
        <small>{{ job.desc }}</small>
    </td>
    <td>{{ job.interval }}</td>
    <td class="item-actions">
      <a @click.prevent="toggle" href="#" class="btn btn-success">Start</a>
      <router-link :to="{ name: 'jobEdit', params: { name: job.name}}" class="btn btn-primary">Edit</router-link>
      <a @click.prevent="toggleModalDelete" href="#" class="btn btn-error">Delete</a>

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
            <a href="#" @click.prevent="run" class="btn btn-primary float-right">Add to queue</a>
          </div>
        </div>
      </div>

      <div class="modal" v-bind:class="{active: modalDelete}">
        <a href="#" @click.prevent="toggleModalDelete" class="modal-overlay" aria-label="Close"></a>
        <div class="modal-container">
          <div class="modal-header">
            <a
              href="#"
              @click.prevent="toggleModalDelete"
              class="btn btn-clear float-right"
              aria-label="Close"
            ></a>
            <div class="modal-title text-uppercase">Delete</div>
          </div>
          <div class="modal-body">
            Confirm to delete
            <b>{{ job.name }}</b>
          </div>
          <div class="modal-footer">
            <a href="#" @click.prevent="deleteJob" class="btn btn-error float-right">Delete</a>
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
            const url =
        `${APIURL}/job/${this.job.name}/run?` +
        new URLSearchParams(
            Array.from(new FormData(this.$refs.form))
        ).toString();
            axios
                .post(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.job.name} has been scheduled (#${response.data})`,
                        type: "success",
                    });
                })
                .catch((error) => {});
        },
        deleteJob(event) {
            const url = `${APIURL}/job/${this.job.name}/`;
            axios
                .delete(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.job.name} has been deleted`,
                        type: "success",
                    });
                    this.toggleModalDelete();
                    this.$router.go();
                })
                .catch((error) => {});
        },
        toggle(event) {
            this.modalOpen = !this.modalOpen;
        },
        toggleModalDelete() {
            this.modalDelete = !this.modalDelete;
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
            modalDelete: false,
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
