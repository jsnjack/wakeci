<template>
  <div class="content">
    <h5>Job syntax</h5>
    <codemirror
      :ref="'codeViewer'"
      class="codemirror-viewer text-left"
      :code="configFormatContent"
      :options="viewerOptions"
    />
  </div>
</template>

<script>
import {codemirror} from "vue-codemirror";
import "codemirror/lib/codemirror.css";
import "codemirror/mode/yaml/yaml.js";
import description from "raw-loader!@/assets/configDescription.yaml";

export default {
    components: {
        codemirror,
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

<style lang="scss">
.CodeMirror {
  height: auto;
}
</style>
