<template>
  <section>
    <div
      class="divider"
      data-content="artifacts"
    />

    <div
      v-if="indexFile"
      class="float-right"
    >
      <a
        :href="indexFile"
        target="_blank"
        class="btn btn-sm"
        data-cy="openIndexFile"
      >Open index.html</a>
    </div>

    <table class="table table-striped table-hover">
      <thead>
        <tr>
          <th
            class="badge c-hand"
            data-cy="artifacts-header-file"
            @click="sortBy('filename')"
          >
            File
          </th>
          <th
            class="badge c-hand"
            data-cy="artifacts-header-size"
            @click="sortBy('size')"
          >
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
          <td>
            <a
              :href="downloadURL(item.filename)"
              target="_blank"
            >
              {{ item.filename }}
            </a>
          </td>
          <td>{{ getSize(item.size) }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<script>

import {humanFileSize} from "@/store/utils";

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
    data: function() {
        return {
            sortOrder: -1,
            sortField: "filename",
        };
    },
    computed: {
        sortedArtifacts: function() {
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
        indexFile: function() {
            // Returns filename of index.html file with shortest filename (in hope that it will be the top-level one)
            let indexEls = this.artifacts.filter(({filename}) => filename.endsWith("index.html"));
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
        sortBy: function(field) {
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
