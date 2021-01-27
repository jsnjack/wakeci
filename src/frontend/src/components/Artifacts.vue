<template>
  <section>
    <div
      class="divider"
      data-content="artifacts"
    />

    <table class="table table-striped table-hover">
      <thead>
        <tr>
          <th>File</th>
          <th>Size</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="item in artifacts"
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
    computed: {
    },
    methods: {
        downloadURL(filename) {
            return `/storage/build/${this.buildID}/artifacts/${filename}`;
        },
        getSize(size) {
            return humanFileSize(size);
        },
    },
};
</script>

<style lang="scss" scoped>
.artifact {
    margin: 0.25em;
}
</style>
