<template>
    <div class="content">
        <Codemirror
            v-model="configFormatContent"
            class="codemirror-viewer text-left"
            :disabled="true"
            :extensions="extensions"
        />
    </div>
</template>

<script>
import description from "@/assets/configDescription.yaml?raw";
import { yaml } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";
import { EditorView } from "@codemirror/view";
import { Codemirror } from "vue-codemirror";
import vuex from "vuex";

export default {
    components: {
        Codemirror,
    },
    computed: {
        ...vuex.mapState(["theme"]),
        extensions() {
            const exts = [yaml(), EditorView.editable.of(false)];
            if (this.theme === "dark") {
                exts.push(oneDark);
            }
            return exts;
        },
    },
    data: function () {
        return {
            configFormatContent: description,
        };
    },
};
</script>
