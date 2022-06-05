<template>
    <div>
        <h3>Edit {{ name }}</h3>
        <br />
        <div>
            <Codemirror
                :value="job.fileContent"
                data-cy="editor"
                :options="editorOptions"
                @input="onCodeChange"
            />
        </div>
        <div class="job-edit-actions">
            <button data-cy="save-button" href="#" class="btn btn-primary" @click.prevent="save">
                Save
            </button>
            <button
                data-cy="save-and-close-button"
                href="#"
                class="btn btn-primary"
                @click.prevent="saveAndClose"
            >
                Save & Close
            </button>
        </div>
    </div>
</template>

<script>
import axios from 'axios';
import Codemirror from 'codemirror-editor-vue3';
import 'codemirror/lib/codemirror.css';
import 'codemirror/mode/yaml/yaml.js';

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
                fileContent: '',
            },
            editorOptions: {
                tabSize: 2,
                mode: 'text/x-yaml',
                lineNumbers: true,
                line: true,
                indentUnit: 2,
            },
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
                .get(`/api/job/${this.name}`)
                .then((response) => {
                    this.job.fileContent = response.data.fileContent || '';
                })
                .catch((error) => {});
        },
        save() {
            const data = new FormData();
            data.append('name', this.job.name);
            data.append('fileContent', this.job.fileContent);
            axios
                .post(`/api/job/${this.name}`, data, {
                    headers: {
                        'Content-type': 'application/x-www-form-urlencoded',
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: 'Saved',
                        type: 'success',
                    });
                })
                .catch((error) => {});
        },
        saveAndClose() {
            this.save();
            this.$router.push('/jobs');
        },
    },
};
</script>

<style lang="scss" scoped>
.CodeMirror {
    @apply h-auto;
}
.job-edit-actions {
    @apply flex gap-2 my-4 justify-end;
}
</style>
