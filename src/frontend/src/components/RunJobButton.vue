<template>
    <button
        :disabled="disabled ? true : null"
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
    <dialog
        :id="'run-job-dialog-' + selectorID"
        ref="dialog"
    >
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
                        text: `${this.jobName} has been scheduled (<a class="inverse-link" href="/build/${response.data}/">#${response.data}</a>)`,
                        type: "primary",
                        duration: 10000,
                    });
                })
                .catch((error) => {});
        },
        toggleModal(event) {
            window.ui("#" + "run-job-dialog-" + this.selectorID);
        },
    },
    mounted: function () {
        this.selectorID = generateRandomString(10);
    },
    data: function () {
        return {
            selectorID: "",
        };
    },
};

function generateRandomString(length) {
    const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
    let result = "";
    for (let i = 0; i < length; i++) {
        result += characters.charAt(Math.floor(Math.random() * characters.length));
    }
    return result;
}
</script>

<style lang="scss" scoped></style>
