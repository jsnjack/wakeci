<template>
  <tr>
    <td>
        <div>{{ job.name }}</div>
        <small>{{ job.desc }}</small>
    </td>
    <td>{{ job.interval }}</td>
    <td>
      <label class="form-switch">
        <input type="checkbox" v-model="isActive" @click.prevent="toggleIsActive"/>
        <i class="form-icon"></i>
      </label>
    </td>
    <td class="actions">
      <RunJobButton v-show="isActive" :params="job.defaultParams" :buttonTitle="'Start'" :jobName="job.name" class="item-action"></RunJobButton>
      <router-link :to="{ name: 'jobEdit', params: { name: job.name}}" class="btn btn-primary item-action">Edit</router-link>
      <a @click.prevent="toggleModalDelete" href="#" class="btn btn-error item-action">Delete</a>

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
import RunJobButton from "@/components/RunJobButton";

export default {
    props: {
        job: {
            type: Object,
            required: true,
        },
    },
    components: {RunJobButton},
    computed: {
    },
    methods: {
        deleteJob(event) {
            const url = `/api/job/${this.job.name}/`;
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
        toggleModalDelete() {
            this.modalDelete = !this.modalDelete;
        },
        toggleIsActive() {
            const url = `/api/job/${this.job.name}/set_active/`;
            const data = new FormData();
            data.append("active", String(!this.isActive));
            axios
                .post(url, data)
                .then((response) => {
                    this.$notify({
                        text: `Job ${this.job.name} is ` + (response.data ? "enabled" : "disabled"),
                        type: "success",
                    });
                    this.isActive = response.data;
                })
                .catch((error) => {});
        },
    },
    data: function() {
        return {
            modalDelete: false,
            isActive: this.job.active === "true",
        };
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.item-action {
    margin: 0.25em;
}
</style>
