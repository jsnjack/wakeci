<template>
    <MoreOptions :optionsList="sortedArtifacts" />
</template>

<script>
import MoreOptions from '@/components/ui/MoreOptions.vue';
import { humanFileSize } from '@/store/utils';

export default {
    components: {
        MoreOptions,
    },
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
            sortField: 'filename',
        };
    },
    computed: {
        sortedArtifacts: function () {
            console.log(this.artifacts);
            return [...this.artifacts]
                .sort((a, b) => {
                    if (a[this.sortField] < b[this.sortField]) {
                        return 1 * this.sortOrder;
                    }
                    if (a[this.sortField] > b[this.sortField]) {
                        return -1 * this.sortOrder;
                    }
                    return 0;
                })
                .map((artifact) => {
                    return {
                        icon: 'cloud_download',
                        name: `${artifact.filename} (${this.getSize(artifact.size)})`,
                        onClick: () => window.open(this.downloadURL(artifact.filename)),
                    };
                });
        },
        indexFile: function () {
            // Returns filename of index.html file with shortest filename (in hope that it will be the top-level one)
            let indexEls = this.artifacts.filter(({ filename }) => filename.endsWith('index.html'));
            indexEls = indexEls.sort((a, b) => {
                return a.filename.length - b.filename.length;
            });
            if (indexEls.length) {
                return this.downloadURL(indexEls[0].filename);
            }
            return '';
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

<style lang="scss" scoped>
.artifact {
    margin: 0.25em;
}
table {
    white-space: pre-wrap;
    word-break: break-word;
}
</style>
