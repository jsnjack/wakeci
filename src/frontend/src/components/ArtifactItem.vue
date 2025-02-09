<template>
    <article v-if="artifacts && artifacts.length > 0">
        <h6>Artifacts</h6>
        <div
            v-if="indexFile"
            class="row"
        >
            <div class="max"></div>
            <a
                :href="indexFile"
                target="_blank"
                class="button secondary"
                data-cy="openIndexFile"
                ><i>open_in_new</i>index.html</a
            >
        </div>

        <table class="large-space large-text stripes">
            <thead>
                <tr>
                    <th
                        style="cursor: pointer"
                        data-cy="artifacts-header-file"
                        @click="sortBy('filename')"
                    >
                        <i>sort</i>
                        File
                    </th>
                    <th
                        style="cursor: pointer"
                        data-cy="artifacts-header-size"
                        @click="sortBy('size')"
                        class="right-align"
                    >
                        <i>sort</i>
                        Size
                    </th>
                </tr>
            </thead>
            <tbody>
                <tr
                    v-for="item in sortedArtifacts"
                    :key="item.path"
                    data-cy="artifacts-body-row"
                >
                    <td style="word-break: break-all">
                        <a
                            :href="downloadURL(item.filename)"
                            target="_blank"
                        >
                            {{ item.filename }}
                        </a>
                    </td>
                    <td class="right-align">{{ getSize(item.size) }}</td>
                </tr>
            </tbody>
        </table>
    </article>
</template>

<script>
import { humanFileSize } from "@/store/utils";

export default {
    props: {
        artifacts: {
            required: true,
            type: Array,
        },
        buildID: {
            required: true,
            type: Number,
        },
    },
    data: function () {
        return {
            sortOrder: -1,
            sortField: "filename",
        };
    },
    computed: {
        sortedArtifacts: function () {
            return [...this.artifacts].sort((a, b) => {
                if (a[this.sortField] < b[this.sortField]) {
                    return 1 * this.sortOrder;
                }
                if (a[this.sortField] > b[this.sortField]) {
                    return -1 * this.sortOrder;
                }
                return 0;
            });
        },
        indexFile: function () {
            // Returns filename of index.html file with shortest filename (in hope that it will be the top-level one)
            let indexEls = this.artifacts.filter(({ filename }) => filename.endsWith("index.html"));
            indexEls = indexEls.sort((a, b) => {
                return a.filename.length - b.filename.length;
            });
            if (indexEls.length) {
                return this.downloadURL(indexEls[0].filename);
            }
            return "";
        },
    },
    methods: {
        downloadURL(filename) {
            return `/storage/build/${this.buildID}/artifacts/${filename}`;
        },
        getSize(size) {
            return humanFileSize(size);
        },
        sortBy: function (field) {
            if (field === this.sortField) {
                this.sortOrder = this.sortOrder === -1 ? 1 : -1;
            } else {
                this.sortField = field;
                this.sortOrder = -1;
            }
        },
    },
};
</script>

<style scoped>
table {
    white-space: pre-wrap;
}
</style>
