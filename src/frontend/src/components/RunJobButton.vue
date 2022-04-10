<template>
    <span>
        <a
            href="#"
            class="btn btn-success"
            @click.prevent="toggleModal"
            >{{ buttonTitle }}</a
        >

        <div
            class="modal"
            :class="{ active: modalOpen }"
        >
            <a
                href="#"
                class="modal-overlay"
                aria-label="Close"
                @click.prevent="toggleModal"
            />
            <div class="modal-container">
                <div class="modal-header">
                    <a
                        href="#"
                        class="btn btn-clear float-right"
                        aria-label="Close"
                        @click.prevent="toggleModal"
                    />
                    <div class="modal-title text-uppercase">{{ getModalTitle }}</div>
                </div>
                <div class="modal-body">
                    <div class="content">
                        <form
                            v-show="params"
                            ref="form"
                        >
                            <RunFormItem
                                v-for="item in params"
                                :key="item.name"
                                :params="item"
                            />
                        </form>
                        <div
                            v-show="!params"
                            class="empty"
                        >
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
            </div>
        </div>
    </span>
</template>

<script>
import RunFormItem from "@/components/RunFormItem.vue";
import axios from "axios";

export default {
    components: { RunFormItem },
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
            const url = `/api/job/${this.jobName}/run?` + new URLSearchParams(Array.from(new FormData(this.$refs.form))).toString();
            axios
                .post(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.jobName} has been scheduled (#<a href="/build/${response.data}/">${response.data}</a>)`,
                        type: "success",
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

<style
    lang="scss"
    scoped
></style>
