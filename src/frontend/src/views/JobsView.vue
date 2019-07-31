<template>
  <div class="container">
    <table class="table table-striped">
      <thead>
        <th class="h-name">Name</th>
        <th class="h-actions">Interval</th>
        <th class="h-actions">Actions</th>
      </thead>
      <tbody>
        <JobItem v-for="item in jobs" :key="item.name" :job="item"></JobItem>
      </tbody>
    </table>

    <div class="empty" v-show="jobs.length === 0">
      <p class="empty-title h5">Empty</p>
    </div>
    <div class="text-center create-section">
      <a href="#" @click.prevent="toggle" class="btn btn-primary">Create new job</a>
      <!-- Modal to create new job -->
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
            <div class="modal-title text-uppercase">Create new job</div>
          </div>
          <div class="modal-body">
            <div class="content text-left">
              <form ref="form">
                <div class="form-group">
                  <label class="form-label" for="new-job-name">Name</label>
                  <input
                    class="form-input"
                    type="text"
                    id="new-job-name"
                    name="new-job-name"
                    v-model="newJobName"
                  />
                </div>
              </form>
            </div>
          </div>
          <div class="modal-footer">
            <a
              href="#"
              @click.prevent="create"
              class="btn btn-primary float-right"
              aria-label="Close"
            >Create</a>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import JobItem from "@/components/JobItem";
import {APIURL} from "@/store/communication";
import axios from "axios";

export default {
    components: {JobItem},
    mounted() {
        this.fetch();
    },
    computed: {
        isModalOpen: function() {
            return this.modalOpen;
        },
    },
    methods: {
        fetch() {
            axios
                .get(APIURL + "/jobs/")
                .then((response) => {
                    this.jobs = response.data || [];
                })
                .catch((error) => {});
        },
        toggle(event) {
            this.modalOpen = !this.modalOpen;
        },
        create() {
            const data = new FormData();
            data.append("name", this.newJobName);
            axios
                .post(`${APIURL}/jobs/create`, data)
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
    },
    data: function() {
        return {
            jobs: [],
            modalOpen: false,
            newJobName: "new_job",
        };
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.create-section {
  margin-top: 1em;
}
.h-actions {
    min-width: 300px;
}
.h-name{
    width: 100%;
}
</style>
