<template>
    <button
        :disabled="disabled"
        class="circle transparent"
        data-cy="run-job-button"
        @click.prevent="toggleModal"
    >
        <i>{{ icon }}</i>
        <div
            v-if="icon === 'play_arrow'"
            class="tooltip bottom"
        >
            Start
        </div>
        <div
            v-if="icon === 'replay'"
            class="tooltip bottom"
        >
            Re-run
        </div>
    </button>
    <dialog :class="{ active: modalOpen }">
        <h5>{{ getModalTitle }}</h5>
        <form
            class="medium-margin"
            v-show="params"
            ref="form"
        >
            <RunFormItem
                v-for="item in params"
                :key="item.name"
                :params="item"
            />
        </form>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="toggleModal"
            >
                Cancel
            </button>
            <button
                data-cy="start-job-confirm"
                @click.prevent="run"
            >
                Add to queue
            </button>
        </nav>
    </dialog>
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
        jobName: {
            type: null,
            required: true,
        },
        disabled: {
            type: Boolean,
            default: false,
            required: false,
        },
        icon: {
            type: String,
            required: false,
            default: "play_arrow",
        },
    },
    data: function () {
        return {
            modalOpen: false,
        };
    },
    computed: {
        getModalTitle: function () {
            return `Configure ${this.jobName}`;
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
                        type: "primary",
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
