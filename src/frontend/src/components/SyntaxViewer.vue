<template>
  <div class="content">
    <h5>Job syntax</h5>
    <Codemirror
      :ref="'codeViewer'"
      :value="configFormatContent"
      class="codemirror-viewer text-left"
      :options="viewerOptions"
    />
  </div>
</template>

<script>
import Codemirror from "codemirror-editor-vue3";
import "codemirror/lib/codemirror.css";
import "codemirror/mode/yaml/yaml.js";
import description from "@/assets/configDescription.yaml?raw";

export default {
    components: {
        Codemirror,
    },
    data: function() {
        return {
            viewerOptions: {
                tabSize: 2,
                mode: "text/x-yaml",
                lineNumbers: false,
                readOnly: true,
            },
            configFormatContent: description,
        };
    },
};
</script>
