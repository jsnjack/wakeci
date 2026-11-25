<template>
    <section
        v-if="artifacts && artifacts.length > 0"
        class="stack gap-2"
    >
        <div class="cluster between">
            <h2>Artifacts</h2>
            <a
                v-if="indexFile"
                :href="indexFile"
                target="_blank"
                rel="noopener noreferrer"
                class="btn"
                data-cy="openIndexFile"
                ><i class="icon small">open_in_new</i>index.html</a
            >
        </div>
        <table class="artifacts">
            <thead>
                <tr>
                    <th
                        data-cy="artifacts-header-file"
                        :aria-sort="sortField === 'filename' ? (sortOrder === -1 ? 'descending' : 'ascending') : 'none'"
                        @click="sortBy('filename')"
                    >
                        File <i class="icon small">swap_vert</i>
                    </th>
                    <th
                        class="size"
                        data-cy="artifacts-header-size"
                        :aria-sort="sortField === 'size' ? (sortOrder === -1 ? 'descending' : 'ascending') : 'none'"
                        @click="sortBy('size')"
                    >
                        Size <i class="icon small">swap_vert</i>
                    </th>
                </tr>
            </thead>
            <tbody>
                <tr
                    v-for="item in sortedArtifacts"
                    :key="item.filename"
                    data-cy="artifacts-body-row"
                >
                    <td class="file">
                        <a
                            :href="downloadURL(item.filename)"
                            target="_blank"
                            rel="noopener noreferrer"
                            >{{ item.filename }}</a
                        >
                    </td>
                    <td class="size nums">{{ getSize(item.size) }}</td>
                </tr>
            </tbody>
        </table>
    </section>
</template>

<script>
import { humanFileSize } from "@/store/utils";

// The files a build kept, sortable by name and size. Artifacts open in a
// new tab without the opener, as the storage's sandbox requires.
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
            // The index.html with the shortest name, likely the top one
            const indexEls = this.artifacts.filter(({ filename }) => filename.endsWith("index.html")).sort((a, b) => a.filename.length - b.filename.length);
            return indexEls.length ? this.downloadURL(indexEls[0].filename) : "";
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
.artifacts {
    inline-size: 100%;
    border-collapse: collapse;
}

.artifacts th,
.artifacts td {
    padding: var(--space-2) var(--space-3);
    border-block-end: 1px solid var(--outline-variant);
    text-align: start;
}

.artifacts th {
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--on-surface-variant);
    cursor: pointer;
    user-select: none;
    white-space: nowrap;
}

.artifacts .size {
    text-align: end;
    white-space: nowrap;
}

.artifacts .file {
    overflow-wrap: anywhere;
}
</style>
