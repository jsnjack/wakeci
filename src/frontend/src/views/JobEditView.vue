<template>
  <div class="container text-left">
    <h4 class="text-center title">
      Edit {{ name }}
    </h4>
    <div>
      <codemirror
        data-cy="editor"
        :code="job.fileContent"
        :options="editorOptions"
        @input="onCodeChange"
      />
    </div>
    <div class="divider" />
    <div class="text-right">
      <button
        class="btn btn-link"
        @click.prevent="toggleHelpModal"
      >
        Show description
      </button>
      <a
        data-cy="save-button"
        href="#"
        class="btn btn-primary"
        @click.prevent="save"
      >Save</a>
      <a
        data-cy="save-and-close-button"
        href="#"
        class="btn btn-primary"
        @click.prevent="saveAndClose"
      >Save & Close</a>
    </div>

    <div
      class="modal modal-lg"
      :class="{active: helpModalOpen}"
    >
      <a
        href="#close"
        class="modal-overlay"
        aria-label="Close"
        @click.prevent="toggleHelpModal"
      />
      <div class="modal-container">
        <div class="modal-header">
          <a
            href="#close"
            class="btn btn-clear float-right"
            aria-label="Close"
            @click.prevent="toggleHelpModal"
          />
          <div class="modal-title text-uppercase">
            Job format description
          </div>
        </div>
        <div class="modal-body">
          <div class="content">
            <codemirror
              :ref="'codeViewer'"
              class="codemirror-viewer"
              :code="configFormatContent"
              :options="viewerOptions"
            />
          </div>
        </div>
        <div class="modal-footer">
          <a
            href="#close"
            class="btn btn-link"
            @click.prevent="toggleHelpModal"
          >Close</a>
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
    components: {
        codemirror,
    },
    props: {
        name: {
            type: String,
            required: true,
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
    mounted() {
        document.title = `Edit ${this.name} - wakeci`;
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
        saveAndClose() {
            this.save();
            this.$router.push("/jobs");
        },
        toggleHelpModal(event) {
            this.helpModalOpen = !this.helpModalOpen;
            this.$refs.codeViewer.refresh();
        },
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
