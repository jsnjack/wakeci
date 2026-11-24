<template>
    <UiDialog
        :open="open"
        :title="`Run ${jobName}`"
        wide
        @update:open="$emit('update:open', $event)"
    >
        <p
            v-if="suggestions.length"
            class="cluster between gap-2 text-sm"
            data-cy="run-suggested"
        >
            <span
                >The build's summary suggests
                <code
                    v-for="field in suggestions"
                    :key="field.name"
                    class="suggested-value"
                    >{{ field.name }}={{ suggested[field.name] }}</code
                ></span
            >
            <button
                type="button"
                class="btn btn-sm"
                data-cy="run-use-suggested"
                @click="useSuggested"
            >
                Use {{ suggestions.length === 1 ? "it" : "them" }}
            </button>
        </p>
        <form
            :id="formID"
            class="run-fields"
            data-cy="run-form"
            @submit.prevent="run"
        >
            <div
                v-for="field in fields"
                :key="field.name"
                class="form-field"
                :class="{ changed: isChanged(field) }"
            >
                <div class="cluster between gap-2">
                    <label
                        class="label-text"
                        :for="`${formID}-${field.name}`"
                    >
                        {{ field.name }}
                        <span
                            v-if="field.label"
                            class="muted"
                        >
                            · label</span
                        >
                    </label>
                    <span
                        v-if="isChanged(field)"
                        class="cluster nowrap gap-1 text-sm"
                    >
                        <span
                            class="changed-mark"
                            data-cy="run-changed"
                            >changed</span
                        >
                        <button
                            type="button"
                            class="btn btn-sm btn-ghost btn-icon"
                            :aria-label="`Back to the default of ${field.name}`"
                            :data-tip="`Back to ${field.default === '' ? 'empty' : field.default}`"
                            @click="values[field.name] = field.default"
                        >
                            <i class="icon small">undo</i>
                        </button>
                    </span>
                </div>
                <label
                    v-if="field.info.type === 'bool'"
                    class="cluster nowrap gap-2"
                >
                    <input
                        :id="`${formID}-${field.name}`"
                        type="checkbox"
                        :checked="isTrue(values[field.name])"
                        :data-cy="`run-param-${field.name}`"
                        @change="values[field.name] = $event.target.checked ? 'true' : 'false'"
                    />
                    {{ isTrue(values[field.name]) ? "true" : "false" }}
                </label>
                <template v-else>
                    <input
                        :id="`${formID}-${field.name}`"
                        v-model="values[field.name]"
                        class="input"
                        :name="field.name"
                        :list="field.info.options?.length ? `${formID}-${field.name}-options` : null"
                        :data-cy="`run-param-${field.name}`"
                    />
                    <datalist
                        v-if="field.info.options?.length"
                        :id="`${formID}-${field.name}-options`"
                    >
                        <option
                            v-for="option in field.info.options"
                            :key="option"
                            :value="option"
                        ></option>
                    </datalist>
                </template>
                <span
                    v-if="field.info.desc"
                    class="hint"
                    >{{ field.info.desc }}</span
                >
                <span
                    v-if="values[field.name] === '' && field.default !== ''"
                    class="hint"
                    data-cy="run-empty-default"
                    >Empty runs the default, {{ field.default }}</span
                >
            </div>
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

// RunDialog asks for a job's parameters and queues a build of it (J2): the
// labels first, what param_info says about each, and which values differ
// from the job's defaults. It remembers no values: they can hold secrets,
// and build artifacts run on wakeci's origin, so they can read and write
// localStorage
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
        // The values to start from, [{ NAME: value }] in the job's order: a
        // build's to run it again, or the job's defaults
        params: {
            type: Array,
            default: () => [],
        },
        // The job's default params as { NAME: value }
        defaults: {
            type: Object,
            default: () => ({}),
        },
        labels: {
            type: Array,
            default: () => [],
        },
        // param_info of the job: { NAME: { desc, options, type } }
        paramInfo: {
            type: Object,
            default: () => ({}),
        },
        // The build this run runs again, if another build started it: the
        // new build stays under the same parent
        again: {
            type: Number,
            default: 0,
        },
        // Values the build's summary suggests, { NAME: value } (summary.js)
        suggested: {
            type: Object,
            default: () => ({}),
        },
    },
    emits: ["update:open"],
    data() {
        return { values: {}, formID: `run-form-${++forms}` };
    },
    computed: {
        // The suggested values the form doesn't have yet
        suggestions() {
            return this.fields.filter((field) => Object.hasOwn(this.suggested, field.name) && this.values[field.name] !== this.suggested[field.name]);
        },
        fields() {
            const fields = this.params.map((param) => {
                const [name, value] = Object.entries(param)[0];
                return {
                    name,
                    value,
                    default: Object.hasOwn(this.defaults, name) ? this.defaults[name] : value,
                    label: this.labels.includes(name),
                    info: this.paramInfo?.[name] || {},
                };
            });
            const rank = (field) => {
                const i = this.labels.indexOf(field.name);
                return i < 0 ? this.labels.length : i;
            };
            return fields.sort((a, b) => rank(a) - rank(b));
        },
    },
    watch: {
        // Each opening starts again
        open: {
            immediate: true,
            handler(value) {
                if (!value) {
                    return;
                }
                this.values = Object.fromEntries(this.fields.map((field) => [field.name, field.value]));
            },
        },
    },
    methods: {
        isChanged(field) {
            return this.values[field.name] !== field.default;
        },
        isTrue(value) {
            return ["true", "1", "yes", "on"].includes(String(value).toLowerCase());
        },
        useSuggested() {
            for (const field of this.suggestions) {
                this.values[field.name] = this.suggested[field.name];
            }
        },
        run() {
            const values = Object.fromEntries(this.fields.map((field) => [field.name, this.values[field.name] ?? ""]));
            this.$emit("update:open", false);
            const query = new URLSearchParams(Object.entries(values));
            if (this.again) {
                query.set("WAKE_AGAIN", this.again);
            }
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

<style scoped>
.suggested-value {
    margin-inline-start: var(--space-1);
    overflow-wrap: anywhere;
}

.run-fields {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(18rem, 100%), 1fr));
    gap: var(--space-4);
}

.changed-mark {
    padding: 0 6px;
    border-radius: 4px;
    background-color: var(--secondary-container);
    color: var(--on-secondary-container);
}

.form-field.changed .input {
    border-color: var(--primary);
}
</style>
