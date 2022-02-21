<template>
  <div class="container grid-xl">
    <table class="table table-striped">
      <thead>
        <th>Name</th>
        <th class="hide-sm">
          Interval
        </th>
        <th class="hide-sm">
          Active
        </th>
        <th>Actions</th>
      </thead>
      <tbody>
        <JobItem
          v-for="item in jobs"
          :key="item.name"
          :job="item"
        />
      </tbody>
    </table>

    <div
      v-show="jobs.length === 0"
      class="empty"
    >
      <p class="empty-title h5">
        Empty
      </p>
    </div>
    <div class="text-center create-section">
      <a
        data-cy="create-job"
        href="#"
        class="btn btn-primary m-1"
        @click.prevent="toggle"
      >Create new job</a>
      <!-- Modal to create new job -->
      <div
        class="modal"
        :class="{active: modalOpen}"
      >
        <a
          href="#"
          class="modal-overlay"
          aria-label="Close"
          @click.prevent="toggle"
        />
        <div class="modal-container">
          <div class="modal-header">
            <a
              href="#"
              class="btn btn-clear float-right"
              aria-label="Close"
              @click.prevent="toggle"
            />
            <div class="modal-title text-uppercase">
              Create new job
            </div>
          </div>
          <div class="modal-body">
            <div class="content text-left">
              <div class="form-group">
                <label
                  class="form-label"
                  for="new-job-name"
                >Name</label>
                <input
                  id="new-job-name"
                  ref="newJobInput"
                  v-model="newJobName"
                  class="form-input"
                  type="text"
                  name="new-job-name"
                  @keyup.enter="enterClicked"
                >
              </div>
            </div>
          </div>
          <div class="modal-footer">
            <a
              ref="createButton"
              data-cy="create-job-button"
              href="#"
              class="btn btn-primary float-right"
              aria-label="Close"
              @click.prevent="create"
            >Create</a>
          </div>
        </div>
      </div>
      <a
        data-cy="refresh-jobs"
        href="#"
        class="btn tooltip m-1"
        data-tooltip="Refresh all jobs from the configuration folder"
        @click.prevent="refreshJobs"
      >Refresh jobs</a>
    </div>
  </div>
</template>

<script>
import JobItem from "@/components/JobItem";
import axios from "axios";

export default {
    components: {JobItem},
    data: function() {
        return {
            jobs: [],
            modalOpen: false,
            newJobName: "new_job",
        };
    },
    computed: {
        isModalOpen: function() {
            return this.modalOpen;
        },
    },
    mounted() {
        document.title = "Jobs - wakeci";
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get("/api/jobs/")
                .then((response) => {
                    this.jobs = response.data || [];
                })
                .catch((error) => {});
        },
        toggle(event) {
            this.modalOpen = !this.modalOpen;
            if (this.modalOpen) {
                this.$nextTick(() => {
                    this.$refs.newJobInput.focus();
                });
            }
        },
        create() {
            const data = new FormData();
            data.append("name", this.newJobName);
            axios
                .post("/api/jobs/create", data)
                .then((response) => {
                    this.toggle();
                    this.$notify({
                        text: "New job created",
                        type: "success",
                    });
                    this.fetch();
                })
                .catch((error) => {});
        },
        enterClicked() {
            this.$refs.createButton.click();
        },
        refreshJobs() {
            // Helps to hide the tooltip
            document.documentElement.focus();
            axios
                .post("/api/jobs/refresh")
                .then((response) => {
                    this.$notify({
                        text: "Jobs have been refreshed",
                        type: "success",
                    });
                    this.fetch();
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.create-section {
  margin-top: 1em;
}
</style>
