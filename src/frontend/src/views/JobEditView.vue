<template>
    <div class="ui job-edit stack gap-3">
        <JobTabs
            :name="name"
            current="edit"
        />
        <div class="editor-box">
            <Codemirror
                v-model="job.fileContent"
                data-cy="editor"
                :autofocus="true"
                :indent-with-tab="true"
                :tab-size="2"
                :extensions="extensions"
            />
        </div>
        <div
            v-if="canAny('write')"
            class="cluster end gap-2"
        >
            <button
                type="button"
                class="btn"
                data-cy="save-button"
                @click="save"
            >
                Save
            </button>
            <button
                type="button"
                class="btn btn-primary"
                data-cy="save-and-close-button"
                @click="saveAndClose"
            >
                Save & Close
            </button>
        </div>
    </div>
</template>

<script>
import JobTabs from "@/components/JobTabs.vue";
import { yaml } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";
import { EditorView } from "@codemirror/view";
import axios from "axios";
import { Codemirror } from "vue-codemirror";
import vuex from "vuex";

export default {
    components: {
        Codemirror,
        JobTabs,
    },
    props: {
        name: {
            type: String,
            required: true,
        },
    },
    data: function () {
        return {
            job: {
                fileContent: "",
            },
        };
    },
    computed: {
        ...vuex.mapState(["theme"]),
        ...vuex.mapGetters(["canAny"]),
        extensions() {
            const exts = [yaml()];
            if (!this.canAny("write")) {
                exts.push(EditorView.editable.of(false));
            }
            if (this.theme === "dark") {
                exts.push(oneDark);
            }
            return exts;
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", `${this.canAny("write") ? "Edit" : "View"} ${this.name}`);
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get(`/api/job/${encodeURIComponent(this.name)}`)
                .then((response) => {
                    this.job.fileContent = response.data.fileContent || "";
                })
                .catch(() => {});
        },
        // save resolves to whether the server took the file; the error itself
        // is shown by axios' interceptor
        save() {
            const data = new FormData();
            data.append("name", this.job.name);
            data.append("fileContent", this.job.fileContent);
            return axios
                .post(`/api/job/${encodeURIComponent(this.name)}`, data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then(() => {
                    this.$notify({
                        text: "Saved",
                        type: "primary",
                    });
                    return true;
                })
                .catch(() => false);
        },
        // Leave only once the file is saved: when the server refuses it, say
        // as invalid YAML, the edit stays in the editor with the error
        async saveAndClose() {
            if (await this.save()) {
                this.$router.push("/jobs");
            }
        },
    },
};
</script>

<style scoped>
.editor-box {
    border: 1px solid var(--outline-variant);
    border-radius: var(--radius-control);
    overflow: hidden;
}
</style>
