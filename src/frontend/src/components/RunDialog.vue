<template>
    <UiDialog
        :open="open"
        :title="`Run ${jobName}`"
        @update:open="$emit('update:open', $event)"
    >
        <form
            :id="formID"
            class="stack gap-3"
            data-cy="run-form"
            @submit.prevent="run"
        >
            <label
                v-for="field in fields"
                :key="field.name"
                class="form-field"
            >
                <span class="label-text">{{ field.name }}</span>
                <input
                    v-model="values[field.name]"
                    class="input"
                    :name="field.name"
                    :data-cy="`run-param-${field.name}`"
                />
            </label>
            <p
                v-if="fields.length === 0"
                class="muted"
            >
                The job has no parameters.
            </p>
        </form>
        <template #actions>
            <button
                type="button"
                class="btn btn-ghost"
                @click="$emit('update:open', false)"
            >
                Cancel
            </button>
            <button
                type="submit"
                class="btn btn-primary"
                :form="formID"
                data-cy="start-job-confirm"
            >
                Add to queue
            </button>
        </template>
    </UiDialog>
</template>

<script>
import UiDialog from "@/components/ui/UiDialog.vue";
import { escapeHtml } from "@/store/utils.js";
import axios from "axios";

let forms = 0;

// RunDialog asks for a job's parameters, filled with params, and queues a
// build of it
export default {
    components: { UiDialog },
    props: {
        open: {
            type: Boolean,
            default: false,
        },
        jobName: {
            type: String,
            required: true,
        },
        // [{ NAME: value }] in the job's order
        params: {
            type: Array,
            default: () => [],
        },
    },
    emits: ["update:open"],
    data() {
        return { values: {}, formID: `run-form-${++forms}` };
    },
    computed: {
        fields() {
            return this.params.map((param) => {
                const [name, value] = Object.entries(param)[0];
                return { name, value };
            });
        },
    },
    watch: {
        // Each opening starts from params
        open: {
            immediate: true,
            handler(value) {
                if (value) {
                    this.values = Object.fromEntries(this.fields.map((field) => [field.name, field.value]));
                }
            },
        },
    },
    methods: {
        run() {
            const query = new URLSearchParams(this.fields.map((field) => [field.name, this.values[field.name] ?? ""]));
            this.$emit("update:open", false);
            axios
                .post(`/api/job/${encodeURIComponent(this.jobName)}/run?${query}`)
                .then((response) => {
                    const id = escapeHtml(response.data);
                    this.$notify({
                        text: `${escapeHtml(this.jobName)} has been scheduled (<a href="/build/${id}/">#${id}</a>)`,
                        type: "primary",
                        duration: 10000,
                    });
                })
                .catch(() => {});
        },
    },
};
</script>
