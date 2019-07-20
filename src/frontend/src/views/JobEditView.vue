<template>
  <div class="container">
    <codemirror class="text-left" :code="code" :options="cmOptions"></codemirror>
  </div>
</template>

<script>
import {APIURL} from "@/store/communication";
import axios from "axios";
import {codemirror} from "vue-codemirror";
import "codemirror/lib/codemirror.css";
import "codemirror/mode/yaml/yaml.js";


export default {
    components: {
        codemirror,
    },
    mounted() {
    // this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get(APIURL + "/jobs/")
                .then((response) => {
                    this.jobs = response.data || [];
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
    },
    data: function() {
        return {
            code: "name: John",
            cmOptions: {
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
