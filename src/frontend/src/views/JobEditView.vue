<template>
  <div class="container text-left">
      <h4 class="text-center title">Edit {{ name }}</h4>
      <div>
        <codemirror :code="job.fileContent" :options="codeMirrorOptions" @input="onCodeChange"></codemirror>
      </div>
      <div class="text-center">
        <a href="#" @click.prevent="save" class="btn btn-primary">Save</a>
      </div>
  </div>
</template>

<script>
import {APIURL} from "@/store/communication";
import axios from "axios";
import {codemirror} from "vue-codemirror";
import "codemirror/lib/codemirror.css";
import "codemirror/mode/yaml/yaml.js";

export default {
    props: {
        name: {
            required: true,
        },
    },
    components: {
        codemirror,
    },
    mounted() {
        this.fetch();
    },
    methods: {
        onCodeChange(newCode) {
            this.job.fileContent = newCode;
        },
        fetch() {
            axios
                .get(APIURL + `/job/${this.name}/`)
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
                .post(APIURL + `/job/${this.name}/`, data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: "Saved",
                        type: "success",
                    });
                })
                .catch((error) => {});
        },
    },
    data: function() {
        return {
            job: {
                fileContent: "",
            },
            codeMirrorOptions: {
                tabSize: 2,
                mode: "text/x-yaml",
                lineNumbers: true,
                line: true,
            },
        };
    },
};
</script>

<style lang="scss">
.CodeMirror {
  height: auto;
}
</style>

<style lang="scss" scoped>
.form-input {
  width: 30%;
}
.title {
    margin-top: 1em;
}
</style>
