<template>
  <tr :data-cy="job.name">
    <td>
      <div>{{ job.name }}</div>
      <small>{{ job.desc }}</small>
    </td>
    <td class="hide-sm">
      {{ job.interval }}
    </td>
    <td class="hide-sm">
      <label class="form-switch">
        <input
          v-model="isActive"
          type="checkbox"
          @click.prevent="toggleIsActive"
        >
        <i class="form-icon" />
      </label>
    </td>
    <td class="actions">
      <RunJobButton
        v-show="isActive"
        :params="job.defaultParams"
        :button-title="'Start'"
        :job-name="job.name"
        class="item-action"
        data-cy="start-job-button"
      />
      <router-link
        :to="{ name: 'jobEdit', params: { name: job.name}}"
        class="btn btn-primary item-action"
        data-cy="edit-job-button"
      >
        Edit
      </router-link>
      <a
        data-cy="delete-job-button"
        href="#"
        class="btn btn-error item-action"
        @click.prevent="toggleModalDelete"
      >Delete</a>

      <div
        class="modal"
        :class="{active: modalDelete}"
      >
        <a
          href="#"
          class="modal-overlay"
          aria-label="Close"
          @click.prevent="toggleModalDelete"
        />
        <div class="modal-container">
          <div class="modal-header">
            <a
              href="#"
              class="btn btn-clear float-right"
              aria-label="Close"
              @click.prevent="toggleModalDelete"
            />
            <div class="modal-title text-uppercase">
              Delete
            </div>
          </div>
          <div class="modal-body">
            Confirm to delete
            <b>{{ job.name }}</b>
          </div>
          <div class="modal-footer">
            <a
              data-cy="delete-job-confirm"
              href="#"
              class="btn btn-error float-right"
              @click.prevent="deleteJob"
            >Delete</a>
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
    components: {RunJobButton},
    props: {
        job: {
            type: Object,
            required: true,
        },
    },
    data: function() {
        return {
            modalDelete: false,
            isActive: this.job.active === "true",
        };
    },
    computed: {},
    methods: {
        deleteJob(event) {
            const url = `/api/job/${this.job.name}`;
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
            const url = `/api/job/${this.job.name}/set_active`;
            const data = new FormData();
            data.append("active", String(!this.isActive));
            axios
                .post(url, data)
                .then((response) => {
                    this.$notify({
                        text:
              `Job ${this.job.name} is ` +
              (response.data ? "enabled" : "disabled"),
                        type: "success",
                    });
                    this.isActive = response.data;
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.item-action {
  margin: 0.25em;
}
</style>
