<template>
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
import { yaml } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";
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
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", `Edit ${this.name}`);
        this.fetch();
    },
    computed: {
        ...vuex.mapState(["theme"]),
        extensions() {
            const exts = [yaml()];
            if (this.theme === "dark") {
                exts.push(oneDark);
            }
            return exts;
        },
    },
    methods: {
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

<style  scoped></style>
