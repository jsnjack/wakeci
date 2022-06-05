<template>
    <tr :data-cy="job.name">
        <td>
            <div>{{ job.name }}</div>
            <small>{{ job.desc }}</small>
        </td>
        <td class="desktop-only">
            {{ job.interval }}
        </td>
        <td class="desktop-only">
            <label class="form-switch">
                <input v-model="isActive" type="checkbox" @click.prevent="toggleIsActive" />
                <i class="form-icon" />
            </label>
        </td>
        <td>
            <div class="actions">
                <RunJobButton
                    v-show="isActive"
                    :params="job.defaultParams"
                    :button-title="'Start'"
                    :job-name="job.name"
                    class="item-action"
                    data-cy="start-job-button"
                />
                <router-link
                    :to="{ name: 'jobEdit', params: { name: job.name } }"
                    class="btn btn-secondary item-action"
                    data-cy="edit-job-button"
                >
                    Edit
                </router-link>
                <a
                    data-cy="delete-job-button"
                    href="#"
                    class="btn btn-danger item-action"
                    @click.prevent="toggleModalDelete"
                    >Delete</a
                >
            </div>

            <Modal @close="modalDelete = false" v-show="modalDelete" title="Confirm to delete">
                <b>{{ job.name }}</b>
                <br />
                <button
                    data-cy="delete-job-confirm"
                    href="#"
                    class="btn btn-error float-right"
                    @click.prevent="deleteJob"
                >
                    Delete
                </button>
            </Modal>
        </td>
    </tr>
</template>

<script>
import axios from 'axios';
import RunJobButton from '@/components/RunJobButton.vue';
import Modal from '@/components/ui/Modal.vue';

export default {
    components: { RunJobButton, Modal },
    props: {
        job: {
            type: Object,
            required: true,
        },
    },
    data: function () {
        return {
            modalDelete: false,
            isActive: this.job.active === 'true',
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
                        type: 'success',
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
            data.append('active', String(!this.isActive));
            axios
                .post(url, data)
                .then((response) => {
                    this.$notify({
                        text: `Job ${this.job.name} is ` + (response.data ? 'enabled' : 'disabled'),
                        type: 'success',
                    });
                    this.isActive = response.data;
                })
                .catch((error) => {});
        },
    },
};
</script>

<style scoped lang="scss">
.actions {
    @apply flex justify-center items-center gap-2 h-full;
}
</style>
