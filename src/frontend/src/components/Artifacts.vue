<template>
  <section>
    <div
      class="divider"
      data-content="artifacts"
    />

    <table class="table table-striped table-hover">
      <thead>
        <tr>
          <th
            class="badge c-hand"
            @click="sortBy('filename')"
          >
            File
          </th>
          <th
            class="badge c-hand"
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
        >
          <td><a :href="downloadURL(item.filename)">{{ item.filename }}</a></td>
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
</style>
