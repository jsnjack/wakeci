<template>
    <dialog :class="{ active: modalOpen }">
        <h5>Create a new job</h5>
        <div class="field border small">
            <input
                id="new-job-name"
                ref="newJobInput"
                v-model="newJobName"
                class="form-input"
                type="text"
                name="new-job-name"
                @keyup.enter="enterClicked"
            />
            <span class="helper">Name</span>
        </div>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="toggle"
            >
                Cancel
            </button>
            <button
                ref="createButton"
                data-cy="create-job-button"
                href="#"
                class="btn btn-primary float-right"
                aria-label="Close"
                @click.prevent="create"
            >
                Create
            </button>
        </nav>
    </dialog>

    <nav class="medium-margin">
        <div class="max"></div>
        <button
            data-cy="create-job"
            @click.prevent="toggle"
        >
            <i>add_circle</i>
            Create a new job
        </button>

        <button
            data-cy="refresh-jobs"
            @click.prevent="refreshJobs"
        >
            <i>sync</i>
            Reload all jobs
        </button>
    </nav>

    <div class="article">
        <JobItem
            v-for="item in jobs"
            :key="item.name"
            :job="item"
        />
        <div
            v-if="jobs.length === 0 && fetchingDone"
            class="fill medium-height middle-align center-align"
        >
            <div class="center-align">
                <i class="extra">water</i>
                <h5>No jobs found</h5>
            </div>
        </div>
    </div>
</template>

<script>
import JobItem from "@/components/JobItem.vue";
import axios from "axios";

export default {
    components: { JobItem },
    data: function () {
        return {
            jobs: [],
            modalOpen: false,
            newJobName: "new_job",
            fetchingDone: false,
        };
    },
    computed: {
        isModalOpen: function () {
            return this.modalOpen;
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Jobs");
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get("/api/jobs/")
                .then((response) => {
                    this.jobs = response.data || [];
                    this.fetchingDone = true;
                })
                .catch((error) => {
                    this.fetchingDone = true;
                });
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
                        type: "primary",
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
                        type: "primary",
                    });
                    this.fetch();
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss"></style>
