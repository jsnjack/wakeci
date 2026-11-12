<template>
    <div class="tabs left-align">
        <a
            class="active"
            data-cy="job-file-tab"
        >
            <i>description</i>
            <span>{{ canAny("write") ? "Edit" : "View" }}</span>
        </a>
        <router-link
            :to="{ name: 'jobInsights', params: { name } }"
            data-cy="job-insights-tab"
        >
            <i>insights</i>
            <span>Insights</span>
        </router-link>
    </div>
    <div class="space"></div>
    <div>
        <Codemirror
            v-model="job.fileContent"
            data-cy="editor"
            :autofocus="true"
            :indent-with-tab="true"
            :tab-size="2"
            :extensions="extensions"
        />
    </div>
    <nav v-if="canAny('write')">
        <div class="max"></div>
        <button
            data-cy="save-button"
            @click.prevent="save"
        >
            Save
        </button>
        <button
            class="secondary"
            data-cy="save-and-close-button"
            @click.prevent="saveAndClose"
        >
            Save & Close
        </button>
    </nav>
</template>

<script>
import { yaml } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";
import { EditorView } from "@codemirror/view";
import axios from "axios";
import { Codemirror } from "vue-codemirror";
import vuex from "vuex";

export default {
    components: {
        Codemirror,
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
        save() {
            const data = new FormData();
            data.append("name", this.job.name);
            data.append("fileContent", this.job.fileContent);
            axios
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
                })
                .catch(() => {});
        },
        saveAndClose() {
            this.save();
            this.$router.push("/jobs");
        },
    },
};
</script>

<style scoped></style>
