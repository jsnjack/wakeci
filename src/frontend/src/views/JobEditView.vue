<template>
  <div class="container text-left">
    <h4 class="text-center title">Edit {{ name }}</h4>
    <div>
      <codemirror :code="job.fileContent" :options="editorOptions" @input="onCodeChange"></codemirror>
    </div>
    <div class="divider"></div>
    <div class="text-right">
      <button @click.prevent="toggleHelpModal" class="btn btn-link">Show description</button>
      <a href="#" @click.prevent="save" class="btn btn-primary">Save</a>
    </div>

    <div class="modal modal-lg" v-bind:class="{active: helpModalOpen}">
      <a href="#close" @click.prevent="toggleHelpModal" class="modal-overlay" aria-label="Close"></a>
      <div class="modal-container">
        <div class="modal-header">
          <a href="#close" @click.prevent="toggleHelpModal" class="btn btn-clear float-right" aria-label="Close"></a>
          <div class="modal-title text-uppercase">Job format description</div>
        </div>
        <div class="modal-body">
          <div class="content">
            <codemirror
              :ref="'codeViewer'"
              class="codemirror-viewer"
              :code="configFormatContent"
              :options="viewerOptions"
            ></codemirror>
          </div>
        </div>
        <div class="modal-footer">
          <a href="#close" @click.prevent="toggleHelpModal" class="btn btn-link">Close</a>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import axios from "axios";
import {codemirror} from "vue-codemirror";
import "codemirror/lib/codemirror.css";
import "codemirror/mode/yaml/yaml.js";
import description from "raw-loader!@/assets/configDescription.yaml";

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
                .get(`/api/job/${this.name}/`)
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
                .post(`/api/job/${this.name}/`, data, {
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
        toggleHelpModal(event) {
            this.helpModalOpen = !this.helpModalOpen;
            this.$refs.codeViewer.refresh();
        },
    },
    data: function() {
        return {
            job: {
                fileContent: "",
            },
            editorOptions: {
                tabSize: 2,
                mode: "text/x-yaml",
                lineNumbers: true,
                line: true,
            },
            viewerOptions: {
                tabSize: 2,
                mode: "text/x-yaml",
                lineNumbers: false,
                readOnly: true,
            },
            configFormatContent: description,
            helpModalOpen: false,
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
@import "@/assets/colors.scss";

.form-input {
  width: 30%;
}
.title {
  margin-top: 1em;
}
.btn {
  margin: 1em;
}
</style>
