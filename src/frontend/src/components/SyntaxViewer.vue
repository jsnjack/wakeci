<template>
    <div class="content">
        <Codemirror
            :value="configFormatContent"
            class="codemirror-viewer text-left"
            :options="getOptions"
        />
    </div>
</template>

<script>
import vuex from "vuex";
import Codemirror from "codemirror-editor-vue3";
import "codemirror/lib/codemirror.css";
import "codemirror/theme/dracula.css";
import "codemirror/mode/yaml/yaml.js";
import description from "@/assets/configDescription.yaml?raw";

export default {
    components: {
        Codemirror,
    },
    computed: {
        ...vuex.mapState(["theme"]),
        getOptions() {
            return {
                tabSize: 2,
                mode: "text/x-yaml",
                lineNumbers: false,
                readOnly: true,
                theme: this.theme === "light" ? "default" : "dracula",
            };
        },
    },
    data: function () {
        return {
            configFormatContent: description,
        };
    },
};
</script>
