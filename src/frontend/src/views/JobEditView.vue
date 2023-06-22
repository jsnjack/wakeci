<template>
    <div>
        <Codemirror
            :value="job.fileContent"
            data-cy="editor"
            :options="getOptions"
            @input="onCodeChange"
        />
    </div>
    <nav>
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
import vuex from "vuex";
import axios from "axios";
import Codemirror from "codemirror-editor-vue3";
import "codemirror/lib/codemirror.css";
import "codemirror/theme/dracula.css";
import "codemirror/mode/yaml/yaml.js";

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
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", `Edit ${this.name}`);
        this.fetch();
    },
    computed: {
        ...vuex.mapState(["theme"]),
        getOptions() {
            return {
                tabSize: 2,
                mode: "text/x-yaml",
                lineNumbers: true,
                line: true,
                indentUnit: 2,
                theme: this.theme === "light" ? "default" : "dracula",
            };
        },
    },
    methods: {
        onCodeChange(newCode) {
            this.job.fileContent = newCode;
        },
        fetch() {
            axios
                .get(`/api/job/${this.name}`)
                .then((response) => {
                    this.job.fileContent = response.data.fileContent || "";
                })
                .catch((error) => {});
        },
        save() {
            const data = new FormData();
            data.append("name", this.job.name);
            data.append("fileContent", this.job.fileContent);
            axios
                .post(`/api/job/${this.name}`, data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: "Saved",
                        type: "primary",
                    });
                })
                .catch((error) => {});
        },
        saveAndClose() {
            this.save();
            this.$router.push("/jobs");
        },
    },
};
</script>

<style lang="scss" scoped></style>
