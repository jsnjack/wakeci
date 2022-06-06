<template>
    <span>
        <a href="#" class="btn btn-success" @click.prevent="toggleModal">{{ buttonTitle }}</a>

        <Modal v-if="modalOpen" @close="toggleModal" :title="getModalTitle">
            <div class="modal-body">
                <div class="content">
                    <form v-show="params" ref="form">
                        <RunFormItem v-for="item in params" :key="item.name" :params="item" />
                    </form>
                    <div v-show="!params" class="empty">
                        <p class="empty-title h6 text-uppercase">Empty</p>
                    </div>
                </div>
            </div>
            <div class="modal-footer">
                <a
                    data-cy="start-job-confirm"
                    href="#"
                    class="btn btn-primary float-right"
                    @click.prevent="run"
                    >Add to queue</a
                >
            </div>
        </Modal>
    </span>
</template>

<script>
import axios from 'axios';
import RunFormItem from './RunFormItem.vue';
import Modal from './ui/Modal.vue';

export default {
    components: { RunFormItem, Modal },
    props: {
        params: {
            type: null,
            required: true,
        },
        buttonTitle: {
            type: String,
            required: true,
        },
        jobName: {
            type: null,
            required: true,
        },
    },
    data: function () {
        return {
            modalOpen: false,
        };
    },
    computed: {
        getModalTitle: function () {
            return `${this.jobName} job parameters`;
        },
    },
    methods: {
        run(event) {
            this.toggleModal();
            const url =
                `/api/job/${this.jobName}/run?` +
                new URLSearchParams(Array.from(new FormData(this.$refs.form))).toString();
            axios
                .post(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.jobName} has been scheduled <p>(Link: <a href="/build/${response.data}/">#${response.data}</a>)</p>`,
                        type: 'success',
                        duration: 10000,
                    });
                })
                .catch((error) => {});
        },
        toggleModal(event) {
            this.modalOpen = !this.modalOpen;
        },
    },
};
</script>

<style lang="scss" scoped></style>
