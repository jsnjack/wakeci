<template>
    <div class="jobs-view">
        <h3>Jobs</h3>
        <div class="jobs-action">
            <button
                data-cy="refresh-jobs"
                href="#"
                class="btn btn-primary"
                title="Refresh all jobs from the configuration folder"
                @click.prevent="refreshJobs"
            >
                Refesh jobs
            </button>

            <button data-cy="create-job" href="#" class="btn btn-success" @click.prevent="toggle">
                Create new job
            </button>
            <input class="job-search" placeholder="Filter..." v-model="search" />
        </div>
        <table class="table jobs-table">
            <thead>
                <th>Name</th>
                <th class="desktop-only">Interval</th>
                <th class="desktop-only">Active</th>
                <th>Actions</th>
            </thead>
            <tbody>
                <JobItem v-for="item in filteredJobs" :key="item.name" :job="item" />
            </tbody>
        </table>

        <div v-show="jobs.length === 0" class="empty">
            <p class="empty-title h5">Empty</p>
        </div>

        <Modal v-if="modalOpen" @close="modalOpen = false" title="Create new job">
            <div class="form-item">
                <label class="form-label" for="new-job-name">Name</label>
                <input
                    id="new-job-name"
                    ref="newJobInput"
                    v-model="newJobName"
                    class="form-input"
                    type="text"
                    name="new-job-name"
                    @keyup.enter="enterClicked"
                />
            </div>

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
        </Modal>
    </div>
</template>

<script>
import axios from 'axios';
import JobItem from '@/components/JobItem.vue';
import Modal from '@/components/ui/Modal.vue';

export default {
    components: { JobItem, Modal },
    data: function () {
        return {
            jobs: [],
            modalOpen: false,
            newJobName: 'new_job',
            search: '',
        };
    },
    computed: {
        isModalOpen: function () {
            return this.modalOpen;
        },
        filteredJobs() {
            return this.jobs.filter((job) => {
                return job.name.toLowerCase().includes(this.search.toLowerCase());
            });
        },
    },
    mounted() {
        document.title = 'Jobs - wakeci';
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get('/api/jobs/')
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
            data.append('name', this.newJobName);
            axios
                .post('/api/jobs/create', data)
                .then((response) => {
                    this.toggle();
                    this.$notify({
                        text: 'New job created',
                        type: 'success',
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
                .post('/api/jobs/refresh')
                .then((response) => {
                    this.$notify({
                        text: 'Jobs have been refreshed',
                        type: 'info',
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
.jobs-view {
    .jobs-action {
        @apply mb-2 flex justify-end items-center gap-2;
        .job-search {
            @apply ml-4;
        }
    }
    .jobs-table {
        @apply w-full;
    }
}
</style>
