<template>
  <div class="container">
    <table class="table table-striped">
      <thead>
        <th>Name</th>
        <th>Actions</th>
      </thead>
      <tbody>
        <JobItem v-for="item in jobs" :key="item.name" :job="item"></JobItem>
      </tbody>
    </table>

    <div class="empty" v-show="jobs.length === 0">
      <p class="empty-title h5">Empty</p>
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
    methods: {
        fetch() {
            axios
                .get(APIURL + "/jobs/")
                .then((response) => {
                    this.jobs = response.data || [];
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
    },
    data: function() {
        return {
            jobs: [],
        };
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
</style>
