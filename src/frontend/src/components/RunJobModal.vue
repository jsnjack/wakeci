<template>
    <Modal @close="$emit('close')" :title="getModalTitle">
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
</template>

<script>
import axios from 'axios';
import Modal from './ui/Modal.vue';
import RunFormItem from './RunFormItem.vue';

export default {
    components: {
        Modal,
        RunFormItem,
    },
    props: {
        params: {
            type: null,
            required: true,
        },
        jobName: {
            type: null,
            required: true,
        },
    },
    computed: {
        getModalTitle: function () {
            return `${this.jobName} job parameters`;
        },
    },
    methods: {
        run(event) {
            const url =
                `/api/job/${this.jobName}/run?` +
                new URLSearchParams(Array.from(new FormData(this.$refs.form))).toString();
            axios
                .post(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.jobName} has been scheduled (#<a href="/build/${response.data}/">${response.data}</a>)`,
                        type: 'success',
                        duration: 10000,
                    });
                })
                .catch((error) => {})
                .finally(() => {
                    this.$emit('close');
                });
        },
    },
};
</script>

<style></style>
