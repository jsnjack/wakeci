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

    <nav class="medium-margin m l">
        <div class="max"></div>
        <button
            data-cy="create-job"
            @click.prevent="toggle"
        >
            <i>add_circle</i>
            Create a new job
        </button>
    </nav>

    <nav class="no-space small-margin">
        <div class="max field label prefix border left-round">
            <i>filter_alt</i>
            <input
                type="text"
                data-cy="filter"
                :value="filter"
                @input="(evt) => (filter = evt.target.value)"
            />
            <label>Filter jobs by name</label>
        </div>
        <button
            class="large right-round secondary"
            @click.prevent="clearFilter"
        >
            <i>backspace</i>
        </button>
    </nav>

    <div
        class="article"
        data-cy="jobs-container"
    >
        <JobItem
            v-for="item in filteredJobs"
            :key="item.name"
            :job="item"
        />
        <div
            v-if="filteredJobs.length === 0 && fetchingDone"
            class="fill medium-height middle-align center-align"
        >
            <div class="center-align">
                <i class="extra">water</i>
                <h5>No jobs found</h5>
            </div>
        </div>
        <div
            v-if="!fetchingDone"
            class="medium-height middle-align center-align"
        >
            <progress class="circle large"></progress>
        </div>
    </div>
</template>

<script>
import JobItem from "@/components/JobItem.vue";
import axios from "axios";
import _ from "lodash";

export default {
    components: { JobItem },
    data: function () {
        return {
            jobs: [],
            filteredJobs: [],
            filter: "",
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
    created() {
        this.filterJobs = _.debounce(() => {
            if (this.filter === "") {
                this.filteredJobs = this.jobs;
                return;
            }

            this.filteredJobs = this.jobs.filter((item) => {
                return item.name.toLowerCase().includes(this.filter.toLowerCase());
            });
        }, 100);
    },
    watch: {
        filter: function () {
            this.filterJobs();
        },
    },
    methods: {
        fetch() {
            axios
                .get("/api/jobs/")
                .then((response) => {
                    this.jobs = response.data || [];
                    this.filteredJobs = this.jobs;
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
        clearFilter() {
            this.filter = "";
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped ></style>
